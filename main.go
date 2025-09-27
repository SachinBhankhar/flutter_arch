// main.go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

	importLine := fmt.Sprintf("import '../features/%s/presentation/pages/%s_page.dart';\n", feature, pageName)

	// route path logic:
	var routePath string
	if pageName == feature {
		routePath = fmt.Sprintf("/%s", pageName)
	} else {
		routePath = fmt.Sprintf("/%s/%s", feature, pageName)
	}

	routeLine := fmt.Sprintf("    GoRoute(path: '%s', builder: (context, state) => %sPage()),\n", routePath, pascalCase(pageName))

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

	// add import if not present
	if !strings.Contains(content, importLine) {
		if strings.Contains(content, "// AUTO_IMPORTS") {
			content = strings.Replace(content, "// AUTO_IMPORTS", importLine+"// AUTO_IMPORTS", 1)
		} else {
			// attempt to insert after last import
			lastImport := strings.LastIndex(content, "import ")
			if lastImport != -1 {
				// find end of the import line
				after := content[lastImport:]
				endLine := strings.Index(after, ";\n")
				if endLine != -1 {
					insertPos := lastImport + endLine + 2
					content = content[:insertPos] + importLine + content[insertPos:]
				} else {
					// fallback: prepend
					content = importLine + content
				}
			} else {
				// no import found, prepend
				content = importLine + "\n" + content
			}
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
			// fallback: try to append routes block before final closing );
			idx := strings.LastIndex(content, ");")
			if idx != -1 {
				block := "\nfinal GoRouter router = GoRouter(\n  routes: [\n" + routeLine + "  ],\n);\n"
				content = content[:idx] + block + content[idx:]
			} else {
				// as last resort, append a router block
				content = content + "\nfinal GoRouter router = GoRouter(\n  routes: [\n" + routeLine + "  ],\n);\n"
			}
		}
		fmt.Printf("➡️  Added GoRoute for %sPage (path: %s)\n", pascalCase(pageName), routePath)
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

// State
class %sState {
  final bool isLoading;
  final String? error;

  %sState({this.isLoading = false, this.error});

  %sState copyWith({bool? isLoading, String? error}) {
    return %sState(
      isLoading: isLoading ?? this.isLoading,
      error: error ?? this.error,
    );
  }
}

// Notifier
class %sNotifier extends StateNotifier<%sState> {
  %sNotifier() : super(%sState());

  void exampleAction() async {
    state = state.copyWith(isLoading: true);
    await Future.delayed(Duration(seconds: 1));
    state = state.copyWith(isLoading: false);
  }
}

// Provider
final %sProvider =
    StateNotifierProvider<%sNotifier, %sState>((ref) => %sNotifier());
`, pascalCase(providerName), pascalCase(providerName), pascalCase(providerName), pascalCase(providerName),
		pascalCase(providerName), pascalCase(providerName), pascalCase(providerName), pascalCase(providerName),
		providerName, pascalCase(providerName), pascalCase(providerName), pascalCase(providerName))

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
  const %sPage({Key? key}) : super(key: key);

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(%sProvider);
    return Scaffold(
      appBar: AppBar(title: Text('%s')),
      body: Center(
        child: state.isLoading
          ? const CircularProgressIndicator()
          : Text('%s Page'),
      ),
    );
  }
}
`, selectedProvider, pascalCase(pageName), pascalCase(pageName), selectedProvider, pascalCase(pageName), pascalCase(pageName))

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
    constLine := fmt.Sprintf("const %s = '%s';\n", constName, pageName)

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
  new datasource <feature> <dsName>`)
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
	default:
		fmt.Println("❌ Unknown command. Use: new")
	}
}

