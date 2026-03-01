// main.go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

var baseStructure = []string{
	"lib/core/error",
	"lib/core/usecases",
	"lib/core/utils",
	"lib/features/%s/data/datasources",
	"lib/features/%s/data/models",
	"lib/features/%s/data/repositories",
	"lib/features/%s/domain/entities",
	"lib/features/%s/domain/repositories",
	"lib/features/%s/domain/usecases",
	"lib/features/%s/presentation/providers",
	"lib/features/%s/presentation/pages",
	"lib/features/%s/presentation/widgets",
	"test/features/%s",
	"test/features/%s/data/datasources",
	"test/features/%s/data/repositories",
	"test/features/%s/domain/entities",
	"test/features/%s/domain/usecases",
	"test/features/%s/presentation/pages",
	"test/features/%s/presentation/providers",
	"test/features/%s/presentation/widgets",
}

func getEnvFromApiManager() (string, error) {
	cmd := exec.Command("find", ".", "-name", "api_manager.dart")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("find command failed: %w", err)
	}
	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", fmt.Errorf("api_manager.dart not found")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", path, err)
	}
	content := string(data)

	// Regex for bool isDebug = true/false;
	reDebug := regexp.MustCompile(`bool\s+isDebug\s*=\s*(true|false)\s*;`)
	reStaging := regexp.MustCompile(`bool\s+isStaging\s*=\s*(true|false)\s*;`)

	isDebug := false
	isStaging := false

	if m := reDebug.FindStringSubmatch(content); len(m) == 2 {
		isDebug = m[1] == "true"
	}
	if m := reStaging.FindStringSubmatch(content); len(m) == 2 {
		isStaging = m[1] == "true"
	}

	switch {
	case isStaging && isDebug:
		return "dev_debug", nil
	case !isStaging && isDebug:
		return "live_debug", nil
	case isStaging && !isDebug:
		return "dev", nil
	default:
		return "live", nil
	}
}

// releaseAPK builds and uploads the APK, then copies the Drive link to clipboard.
func releaseAPK(clean bool) {
	apkPath, err := buildFlutterAPK(clean)
	if err != nil {
		log.Fatalf("Error building APK: %v", err)
	}
	fmt.Printf("APK built at: %s\n", apkPath)
	url, err := uploadToDrive(apkPath)
	if err != nil {
		log.Fatalf("Error uploading to Google Drive: %v", err)
	}
	fmt.Printf("APK uploaded to: %s\n", url)
	if err := copyToClipboard(url); err != nil {
		fmt.Printf("Failed to copy URL to clipboard: %v\n", err)
	} else {
		fmt.Println("URL copied to clipboard!")
	}
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// buildFlutterAPK builds the APK and returns the output path.
func buildFlutterAPK(clean bool) (string, error) {
	if clean {
		if err := runCommand("flutter", "clean"); err != nil {
			log.Printf("flutter clean failed: %v", err)
		}
		if err := runCommand("flutter", "pub", "get"); err != nil {
			log.Printf("flutter pub get failed: %v", err)
		}
	}

	branchNameCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	branchBytes, err := branchNameCmd.Output()
	if err != nil {
		return "", err
	}
	branchName := strings.TrimSpace(string(branchBytes))
	dateStr := time.Now().Format("20060102")

	envType, err := getEnvFromApiManager()

	if err != nil {
		envType = ""
	}

	apkPath := fmt.Sprintf("builds/lkp/livekeeping_%s_%s_%s.apk", dateStr, branchName, envType)

	if err := runCommand("flutter", "build", "apk"); err != nil {
		return "", err
	}

	apkSrc := "build/app/outputs/flutter-apk/app-arm64-v8a-release.apk"
	if _, err := os.Stat(apkSrc); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(apkPath), 0755); err != nil {
		return "", err
	}

	in, err := os.Open(apkSrc)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := os.Create(apkPath)
	if err != nil {
		return "", err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return "", err
	}
	return apkPath, nil
}

func getDriveService() (*drive.Service, error) {
	ctx := context.Background()
	// b, err := os.ReadFile("~/credentials.json")

	j := []byte(``)

	// if err != nil {
	// 	return nil, err
	// }
	config, err := google.ConfigFromJSON(j, drive.DriveFileScope)
	if err != nil {
		return nil, err
	}
	tokFile := filepath.Join(os.Getenv("HOME"), ".token.json")
	var tok *oauth2.Token
	if f, err := os.Open(tokFile); err == nil {
		defer f.Close()
		tok = &oauth2.Token{}
		if err := json.NewDecoder(f).Decode(tok); err != nil {
			tok = nil
		}
	}
	if tok == nil || !tok.Valid() {
		authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
		fmt.Printf("Go to the following link in your browser then type the authorization code:\n%v\n", authURL)
		fmt.Print("Enter authorization code: ")
		var code string
		fmt.Scanln(&code)
		tok, err = config.Exchange(ctx, code)
		if err != nil {
			return nil, err
		}
		f, err := os.Create(tokFile)
		if err == nil {
			defer f.Close()
			json.NewEncoder(f).Encode(tok)
		}
	}
	client := config.Client(ctx, tok)
	return drive.NewService(ctx, option.WithHTTPClient(client))
}

// uploadToDrive uploads the file and returns the file URL.
func uploadToDrive(filePath string) (string, error) {
	driveService, err := getDriveService()
	if err != nil {
		return "", err
	}
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	fileName := filepath.Base(filePath)
	file := &drive.File{
		Name:     fileName,
		MimeType: "application/vnd.android.package-archive", // Force APK MIME type
	}
	uploaded, err := driveService.Files.Create(file).Media(f).Do()
	if err != nil {
		return "", err
	}
	// Make file public
	_, err = driveService.Permissions.Create(uploaded.Id, &drive.Permission{
		Role:   "reader",
		Type:   "domain",
		Domain: "livekeeping.com",
	}).Do()
	if err != nil {
		return "", err
	}
	url := "https://drive.google.com/file/d/" + uploaded.Id + "/view?usp=sharing"
	return url, nil
}

// copyToClipboard copies the given text to the macOS clipboard.
func copyToClipboard(text string) error {
	cmd := exec.Command("pbcopy")
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	_, err = in.Write([]byte(text))
	if err != nil {
		return err
	}
	in.Close()
	return cmd.Wait()
}

func camelCase(s string) string {
	parts := strings.Split(s, "_")
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.ToLower(parts[0]) + strings.Join(parts[1:], "")
}

// pascalCase turns "my_page" or "my-page" into "MyPage"
func pascalCase(s string) string {
	if s == "" {
		return ""
	}
	// normalize separators to underscore
	s = strings.ReplaceAll(s, "-", "_")
	// split by underscores and spaces
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '_' || r == ' ' || r == '/' || r == '\\'
	})
	for i, p := range parts {
		if p == "" {
			continue
		}
		if len(p) == 1 {
			parts[i] = strings.ToUpper(p)
		} else {
			parts[i] = strings.ToUpper(string(p[0])) + strings.ToLower(p[1:])
		}
	}
	return strings.Join(parts, "")
}

func insertImportDirective(content, importLine string) string {
	if strings.Contains(content, importLine) {
		return content
	}

	lines := strings.Split(content, "\n")
	insertAt := 0
	lastImport := -1
	firstPart := -1
	libraryLine := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "library ") {
			libraryLine = i
		}
		if strings.HasPrefix(trimmed, "import ") {
			lastImport = i
		}
		if firstPart == -1 && strings.HasPrefix(trimmed, "part ") {
			firstPart = i
		}
	}

	switch {
	case lastImport != -1:
		insertAt = lastImport + 1
	case firstPart != -1:
		insertAt = firstPart
	case libraryLine != -1:
		insertAt = libraryLine + 1
	default:
		insertAt = 0
	}

	newLines := make([]string, 0, len(lines)+1)
	newLines = append(newLines, lines[:insertAt]...)
	newLines = append(newLines, strings.TrimRight(importLine, "\n"))
	newLines = append(newLines, lines[insertAt:]...)
	return strings.Join(newLines, "\n")
}

func normalizeImportPartOrder(content string) string {
	lines := strings.Split(content, "\n")
	firstPart := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "part ") {
			firstPart = i
			break
		}
	}
	if firstPart == -1 {
		return content
	}

	misplacedImports := make([]string, 0)
	filtered := make([]string, 0, len(lines))
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if i > firstPart && strings.HasPrefix(trimmed, "import ") {
			misplacedImports = append(misplacedImports, line)
			continue
		}
		filtered = append(filtered, line)
	}
	if len(misplacedImports) == 0 {
		return content
	}

	firstPartFiltered := -1
	lastImportBeforePart := -1
	for i, line := range filtered {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "part ") && firstPartFiltered == -1 {
			firstPartFiltered = i
		}
		if firstPartFiltered == -1 && strings.HasPrefix(trimmed, "import ") {
			lastImportBeforePart = i
		}
	}
	if firstPartFiltered == -1 {
		return content
	}

	insertAt := firstPartFiltered
	if lastImportBeforePart != -1 {
		insertAt = lastImportBeforePart + 1
	}

	out := make([]string, 0, len(filtered)+len(misplacedImports))
	out = append(out, filtered[:insertAt]...)
	out = append(out, misplacedImports...)
	out = append(out, filtered[insertAt:]...)
	return strings.Join(out, "\n")
}

// appendRoute ensures router.dart exists, adds an import for the page and injects a GoRoute
func appendRoute(feature, pageName string) {
	routerFile := filepath.Join("lib", "core", "router.dart")

	// ensure folder exists
	_ = os.MkdirAll(filepath.Dir(routerFile), os.ModePerm)

	// import page
	importLine := fmt.Sprintf("import '../features/%s/presentation/pages/%s_page.dart';\n", feature, pageName)
	// import page_names.dart constants
	constImport := "import 'page_names.dart';\n"

	// route constant
	constName := "k" + pascalCase(pageName) + "Page"

	routeLine := fmt.Sprintf("    GoRoute(path: %s, builder: (context, state) => const %sPage()),\n", constName, pascalCase(pageName))

	// create base router if doesn't exist
	if _, err := os.Stat(routerFile); os.IsNotExist(err) {
		base := `import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:riverpod_annotation/riverpod_annotation.dart';
part 'router.g.dart';

// REQUIRED_FOR_FARCH: Do not remove this marker. farch inserts page imports above this line.
// AUTO_IMPORTS

@riverpod
Raw<GoRouter> router(Ref ref) => GoRouter(
  routes: [
    // REQUIRED_FOR_FARCH: Do not remove this marker. farch inserts new GoRoute entries above this line.
    // AUTO_ROUTES
  ],
);
`
		if err := os.WriteFile(routerFile, []byte(base), 0644); err != nil {
			fmt.Printf("❌ Failed to create %s: %v\n", routerFile, err)
			return
		}
		fmt.Println("📝 Created lib/core/router.dart")
	}

	// read file
	data, err := os.ReadFile(routerFile)
	if err != nil {
		fmt.Printf("❌ Failed to read %s: %v\n", routerFile, err)
		return
	}
	content := string(data)
	content = normalizeImportPartOrder(content)

	// add import for page_names.dart if not present
	if !strings.Contains(content, constImport) {
		if !strings.Contains(content, "// AUTO_IMPORTS") {
			fmt.Println("⚠️  router.dart is missing // AUTO_IMPORTS marker; using fallback import insertion.")
		}
		content = insertImportDirective(content, constImport)
		fmt.Println("➡️  Added import for page_names.dart")
	}

	// add import for page if not present
	if !strings.Contains(content, importLine) {
		if !strings.Contains(content, "// AUTO_IMPORTS") {
			fmt.Println("⚠️  router.dart is missing // AUTO_IMPORTS marker; using fallback import insertion.")
		}
		content = insertImportDirective(content, importLine)
		fmt.Printf("➡️  Added import for %s_page.dart\n", pageName)
	}

	// add route if not present
	if !strings.Contains(content, routeLine) {
		if strings.Contains(content, "// AUTO_ROUTES") {
			content = strings.Replace(content, "// AUTO_ROUTES", routeLine+"    // AUTO_ROUTES", 1)
		} else if strings.Contains(content, "routes: [") {
			fmt.Println("⚠️  router.dart is missing // AUTO_ROUTES marker; inserting route at start of routes list.")
			content = strings.Replace(content, "routes: [", "routes: [\n"+routeLine, 1)
		} else {
			fmt.Println("⚠️  router.dart structure is non-standard; appending fallback GoRouter block.")
			idx := strings.LastIndex(content, ");")
			if idx != -1 {
				block := "\nfinal GoRouter router = GoRouter(\n  routes: [\n" + routeLine + "  ],\n);\n"
				content = content[:idx] + block + content[idx:]
			} else {
				content = content + "\nfinal GoRouter router = GoRouter(\n  routes: [\n" + routeLine + "  ],\n);\n"
			}
		}
		fmt.Printf("➡️  Added GoRoute for %sPage (path: %s)\n", pascalCase(pageName), constName)
	}

	// write back
	if err := os.WriteFile(routerFile, []byte(content), 0644); err != nil {
		fmt.Printf("❌ Failed to update %s: %v\n", routerFile, err)
		return
	}
}

// createFeature scaffolds the whole feature folders and some default files
func createFeature(feature string) {
	for _, pattern := range baseStructure {
		var dirPath string
		if strings.Contains(pattern, "%s") {
			dirPath = fmt.Sprintf(pattern, feature)
		} else {
			dirPath = pattern
		}
		_ = os.MkdirAll(dirPath, os.ModePerm)
		fmt.Printf("✅ Created %s\n", dirPath)
	}
	// default scaffolds
	createEntity(feature, feature)
	createUsecase(feature, "example")
	createRepository(feature, feature)
	createDatasource(feature, "remote")
	createProvider(feature, feature)
	createPage(feature, feature)

	rootFile := filepath.Join("lib", "injection_container.dart")
	if _, err := os.Stat(rootFile); os.IsNotExist(err) {
		if err := os.WriteFile(rootFile, []byte("// Dependency injection setup\n"), 0644); err == nil {
			fmt.Println("📝 Created lib/injection_container.dart")
		}
	}
}

func createEntity(feature, entityName string) {
	dir := filepath.Join("lib", "features", feature, "domain", "entities")
	_ = os.MkdirAll(dir, os.ModePerm)

	file := filepath.Join(dir, entityName+".dart")
	content := fmt.Sprintf(`class %s {
  final int id;
  %s(this.id);
}
`, pascalCase(entityName), pascalCase(entityName))

	writeFile(file, content)
	writeTest(feature, filepath.Join("domain", "entities", entityName+"_entity_test.dart"), testStub("entity "+entityName))
}

func createUsecase(feature, usecaseName string) {
	dir := filepath.Join("lib", "features", feature, "domain", "usecases")
	_ = os.MkdirAll(dir, os.ModePerm)

	file := filepath.Join(dir, usecaseName+".dart")
	content := fmt.Sprintf(`class %s {
  Future<void> call() async {
    // TODO: implement usecase
  }
}
`, pascalCase(usecaseName))

	writeFile(file, content)
	writeTest(feature, filepath.Join("domain", "usecases", usecaseName+"_usecase_test.dart"), testStub("usecase "+usecaseName))
}

func createRepository(feature, repoName string) {
	domainDir := filepath.Join("lib", "features", feature, "domain", "repositories")
	dataDir := filepath.Join("lib", "features", feature, "data", "repositories")

	_ = os.MkdirAll(domainDir, os.ModePerm)
	_ = os.MkdirAll(dataDir, os.ModePerm)

	domainFile := filepath.Join(domainDir, repoName+"_repository.dart")
	dataFile := filepath.Join(dataDir, repoName+"_repository_impl.dart")

	domainContent := fmt.Sprintf(`abstract class %sRepository {
  // TODO: define repository methods
}
`, pascalCase(repoName))

	dataContent := fmt.Sprintf(`import '../../domain/repositories/%s_repository.dart';

class %sRepositoryImpl implements %sRepository {
  // TODO: implement methods
}
`, repoName, pascalCase(repoName), pascalCase(repoName))

	writeFile(domainFile, domainContent)
	writeFile(dataFile, dataContent)
	writeTest(feature, filepath.Join("data", "repositories", repoName+"_repository_test.dart"), testStub("repository "+repoName))
}

func createDatasource(feature, dsName string) {
	dir := filepath.Join("lib", "features", feature, "data", "datasources")
	_ = os.MkdirAll(dir, os.ModePerm)

	file := filepath.Join(dir, dsName+"_datasource.dart")
	content := fmt.Sprintf(`abstract class %sDataSource {
  // TODO: define data source methods
}

class %sDataSourceImpl implements %sDataSource {
  // TODO: implement data source
}
`, pascalCase(dsName), pascalCase(dsName), pascalCase(dsName))

	writeFile(file, content)
	writeTest(feature, filepath.Join("data", "datasources", dsName+"_datasource_test.dart"), testStub("datasource "+dsName))
}

func createProvider(feature, providerName string) {
	dir := filepath.Join("lib", "features", feature, "presentation", "providers")
	_ = os.MkdirAll(dir, os.ModePerm)

	file := filepath.Join(dir, providerName+"_provider.dart")
	// provider variable uses lower-case providerName + "Provider"
	content := fmt.Sprintf(`import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:riverpod_annotation/riverpod_annotation.dart';
part '%s_provider.g.dart';

@riverpod
int %s(Ref ref) => 0;
`,
		providerName,            // %s -> generated part file name
		camelCase(providerName), // %s -> provider variable name
	)

	writeFile(file, content)
	writeTest(feature, filepath.Join("presentation", "providers", providerName+"_provider_test.dart"), testStub("provider "+providerName))
}

func createPage(feature, pageName string) {
	dir := filepath.Join("lib", "features", feature, "presentation", "pages")
	_ = os.MkdirAll(dir, os.ModePerm)

	file := filepath.Join(dir, pageName+"_page.dart")

	// Decide provider to import:
	// prefer a provider named after the page if it exists, else fall back to feature provider
	selectedProvider := pageName
	providerPath := filepath.Join("lib", "features", feature, "presentation", "providers", selectedProvider+"_provider.dart")
	if _, err := os.Stat(providerPath); os.IsNotExist(err) {
		// fallback to feature-named provider
		selectedProvider = feature
		providerPath = filepath.Join("lib", "features", feature, "presentation", "providers", selectedProvider+"_provider.dart")
		if _, fallbackErr := os.Stat(providerPath); os.IsNotExist(fallbackErr) {
			createProvider(feature, selectedProvider)
		}
	}
	selectedProviderVar := camelCase(selectedProvider)

	content := fmt.Sprintf(`import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../providers/%s_provider.dart';

class %sPage extends ConsumerWidget {
  const %sPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(%sProvider);
    return Scaffold(
      appBar: AppBar(title: Text('%s')),
      body: Center(),
    );
  }
}
`, selectedProvider, pascalCase(pageName), pascalCase(pageName), selectedProviderVar, pascalCase(pageName))

	writeFile(file, content)
	writeTest(feature, filepath.Join("presentation", "pages", pageName+"_page_test.dart"), testStub("page "+pageName))

	addPageConstant(pageName)
	// Add route to router.dart
	appendRoute(feature, pageName)
}

func writeFile(path, content string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte(content), 0644); err == nil {
			fmt.Printf("📝 Created %s\n", path)
		} else {
			fmt.Printf("❌ Failed writing %s: %v\n", path, err)
		}
	} else {
		fmt.Printf("⚠️  File %s already exists (skipped)\n", path)
	}
}

func testStub(subject string) string {
	return fmt.Sprintf(`import 'package:flutter_test/flutter_test.dart';

void main() {
  test('%s scaffold placeholder', () {
    expect(true, isTrue);
  });
}
`, subject)
}

func writeTest(feature, filename, content string) {
	path := filepath.Join("test", "features", feature, filename)
	_ = os.MkdirAll(filepath.Dir(path), os.ModePerm)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte(content), 0644); err == nil {
			fmt.Printf("🧪 Created test %s\n", path)
		} else {
			fmt.Printf("❌ Failed writing test %s: %v\n", path, err)
		}
	}
}

func classifyLegacyTest(name string) string {
	switch {
	case strings.HasSuffix(name, "_entity_test.dart"):
		return filepath.Join("domain", "entities")
	case strings.HasSuffix(name, "_usecase_test.dart"):
		return filepath.Join("domain", "usecases")
	case strings.HasSuffix(name, "_repository_test.dart"):
		return filepath.Join("data", "repositories")
	case strings.HasSuffix(name, "_datasource_test.dart"):
		return filepath.Join("data", "datasources")
	case strings.HasSuffix(name, "_provider_test.dart"):
		return filepath.Join("presentation", "providers")
	case strings.HasSuffix(name, "_page_test.dart"):
		return filepath.Join("presentation", "pages")
	case strings.HasSuffix(name, "_widget_test.dart"):
		return filepath.Join("presentation", "widgets")
	default:
		return ""
	}
}

func migrateLegacyTests() {
	root := filepath.Join("test", "features")
	features, err := os.ReadDir(root)
	if err != nil {
		fmt.Printf("❌ Failed to read %s: %v\n", root, err)
		return
	}

	moved := 0
	skipped := 0
	for _, feature := range features {
		if !feature.IsDir() {
			continue
		}

		featureDir := filepath.Join(root, feature.Name())
		entries, err := os.ReadDir(featureDir)
		if err != nil {
			fmt.Printf("❌ Failed to read %s: %v\n", featureDir, err)
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			name := entry.Name()
			targetRelDir := classifyLegacyTest(name)
			if targetRelDir == "" {
				skipped++
				continue
			}

			src := filepath.Join(featureDir, name)
			dst := filepath.Join(featureDir, targetRelDir, name)
			if _, err := os.Stat(dst); err == nil {
				fmt.Printf("⚠️  Target exists, skipped: %s\n", dst)
				skipped++
				continue
			}

			_ = os.MkdirAll(filepath.Dir(dst), os.ModePerm)
			if err := os.Rename(src, dst); err != nil {
				fmt.Printf("❌ Failed to move %s -> %s: %v\n", src, dst, err)
				skipped++
				continue
			}
			fmt.Printf("✅ Moved %s -> %s\n", src, dst)
			moved++
		}
	}

	fmt.Printf("🎯 Migration complete. moved=%d skipped=%d\n", moved, skipped)
}

func addPageConstant(pageName string) {
	constFile := filepath.Join("lib", "core", "page_names.dart")
	_ = os.MkdirAll(filepath.Dir(constFile), os.ModePerm)

	constName := "k" + pascalCase(pageName) + "Page"
	constLine := fmt.Sprintf("const %s = '/%s';\n", constName, pageName)

	// Create file with header if missing
	if _, err := os.Stat(constFile); os.IsNotExist(err) {
		header := "// AUTO_GENERATED – do not edit manually.\n\n"
		if err := os.WriteFile(constFile, []byte(header), 0644); err != nil {
			fmt.Printf("❌ Failed to create %s: %v\n", constFile, err)
			return
		}
	}

	// Read existing
	data, err := os.ReadFile(constFile)
	if err != nil {
		fmt.Printf("❌ Failed to read %s: %v\n", constFile, err)
		return
	}
	content := string(data)

	// Only add if it doesn’t already exist
	if !strings.Contains(content, constName) {
		if err := os.WriteFile(constFile, append(data, []byte(constLine)...), 0644); err != nil {
			fmt.Printf("❌ Failed to update %s: %v\n", constFile, err)
			return
		}
		fmt.Printf("🔗 Added constant %s to page_names.dart\n", constName)
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println(`Usage:
  new feature <name>
  new page <feature> <pageName>
  new provider <feature> <providerName>
  new entity <feature> <entityName>
  new usecase <feature> <usecaseName>
  new repository <feature> <repoName>
  new datasource <feature> <dsName>
  migrate tests
  release apk`)
		return
	}

	cmd := os.Args[1]
	switch cmd {
	case "new":
		if len(os.Args) < 4 {
			fmt.Println("❌ missing arguments for 'new'")
			return
		}
		subCmd := os.Args[2]
		switch subCmd {
		case "feature":
			name := strings.ToLower(os.Args[3])
			createFeature(name)
		case "page":
			if len(os.Args) < 5 {
				fmt.Println("❌ new page requires <feature> <pageName>")
				return
			}
			feature := strings.ToLower(os.Args[3])
			page := strings.ToLower(os.Args[4])
			createPage(feature, page)
		case "provider":
			if len(os.Args) < 5 {
				fmt.Println("❌ new provider requires <feature> <providerName>")
				return
			}
			feature := strings.ToLower(os.Args[3])
			provider := strings.ToLower(os.Args[4])
			createProvider(feature, provider)
		case "entity":
			if len(os.Args) < 5 {
				fmt.Println("❌ new entity requires <feature> <entityName>")
				return
			}
			feature := strings.ToLower(os.Args[3])
			entity := strings.ToLower(os.Args[4])
			createEntity(feature, entity)
		case "usecase":
			if len(os.Args) < 5 {
				fmt.Println("❌ new usecase requires <feature> <usecaseName>")
				return
			}
			feature := strings.ToLower(os.Args[3])
			usecase := strings.ToLower(os.Args[4])
			createUsecase(feature, usecase)
		case "repository":
			if len(os.Args) < 5 {
				fmt.Println("❌ new repository requires <feature> <repoName>")
				return
			}
			feature := strings.ToLower(os.Args[3])
			repo := strings.ToLower(os.Args[4])
			createRepository(feature, repo)
		case "datasource":
			if len(os.Args) < 5 {
				fmt.Println("❌ new datasource requires <feature> <dsName>")
				return
			}
			feature := strings.ToLower(os.Args[3])
			ds := strings.ToLower(os.Args[4])
			createDatasource(feature, ds)
		default:
			fmt.Println("❌ Unknown subcommand. Use: feature | page | provider | entity | usecase | repository | datasource")
		}
	case "migrate":
		if len(os.Args) < 3 {
			fmt.Println("❌ missing arguments for 'migrate'")
			return
		}
		subCmd := os.Args[2]
		switch subCmd {
		case "tests":
			migrateLegacyTests()
		default:
			fmt.Println("❌ Unknown migrate subcommand. Use: tests")
		}
	case "release":
		if len(os.Args) < 3 {
			fmt.Println("❌ missing arguments for 'release'")
			return
		}
		subCmd := os.Args[2]
		switch subCmd {
		case "capk":
			releaseAPK(true)
		case "apk":
			releaseAPK(false)
		default:
			fmt.Println("❌ Unknown release subcommand. Use: upload-apk")
		}
	default:
		fmt.Println("❌ Unknown command. Use: new")
	}
}
