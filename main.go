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

	routeLine := fmt.Sprintf("    GoRoute(path: %s, builder: (context, state) => %sPage()),\n", constName, pascalCase(pageName))

	// create base router if doesn't exist
	if _, err := os.Stat(routerFile); os.IsNotExist(err) {
		base := `import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
// AUTO_IMPORTS

final GoRouter router = GoRouter(
  routes: [
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

	// add import for page_names.dart if not present
	if !strings.Contains(content, constImport) {
		if strings.Contains(content, "// AUTO_IMPORTS") {
			content = strings.Replace(content, "// AUTO_IMPORTS", constImport+"// AUTO_IMPORTS", 1)
		} else {
			content = constImport + content
		}
		fmt.Println("➡️  Added import for page_names.dart")
	}

	// add import for page if not present
	if !strings.Contains(content, importLine) {
		if strings.Contains(content, "// AUTO_IMPORTS") {
			content = strings.Replace(content, "// AUTO_IMPORTS", importLine+"// AUTO_IMPORTS", 1)
		} else {
			content = importLine + content
		}
		fmt.Printf("➡️  Added import for %s_page.dart\n", pageName)
	}

	// add route if not present
	if !strings.Contains(content, routeLine) {
		if strings.Contains(content, "// AUTO_ROUTES") {
			content = strings.Replace(content, "// AUTO_ROUTES", routeLine+"    // AUTO_ROUTES", 1)
		} else if strings.Contains(content, "routes: [") {
			content = strings.Replace(content, "routes: [", "routes: [\n"+routeLine, 1)
		} else {
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
	writeTest(feature, entityName+"_entity_test.dart", "// TODO: write entity tests\n")
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
	writeTest(feature, usecaseName+"_usecase_test.dart", "// TODO: write usecase tests\n")
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
	writeTest(feature, repoName+"_repository_test.dart", "// TODO: write repository tests\n")
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
	writeTest(feature, dsName+"_datasource_test.dart", "// TODO: write datasource tests\n")
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
		feature,                 // %s -> class name
		camelCase(providerName), // %s -> provider variable name
	)

	writeFile(file, content)
	writeTest(feature, providerName+"_provider_test.dart", "// TODO: write provider tests\n")
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
	}

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
`, selectedProvider, pascalCase(pageName), pascalCase(pageName), selectedProvider, pascalCase(pageName))

	writeFile(file, content)
	writeTest(feature, pageName+"_page_test.dart", "// TODO: write page widget tests\n")

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

func writeTest(feature, filename, content string) {
	dir := filepath.Join("test", "features", feature)
	_ = os.MkdirAll(dir, os.ModePerm)
	path := filepath.Join(dir, filename)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte(content), 0644); err == nil {
			fmt.Printf("🧪 Created test %s\n", path)
		} else {
			fmt.Printf("❌ Failed writing test %s: %v\n", path, err)
		}
	}
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
	if len(os.Args) < 3 {
		fmt.Println(`Usage:
  new feature <name>
  new page <feature> <pageName>
  new provider <feature> <providerName>
  new entity <feature> <entityName>
  new usecase <feature> <usecaseName>
  new repository <feature> <repoName>
  new datasource <feature> <dsName>
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
