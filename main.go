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

func pascalCase(s string) string {
	parts := strings.Split(s, "_")
	for i := range parts {
		parts[i] = strings.Title(parts[i])
	}
	return strings.Join(parts, "")
}

func createFeature(feature string) {
	for _, path := range baseStructure {
		dirPath := fmt.Sprintf(path, feature)
		_ = os.MkdirAll(dirPath, os.ModePerm)
		fmt.Printf("✅ Created %s\n", dirPath)
	}

	createEntity(feature, feature)
	createUsecase(feature, "example")
	createRepository(feature, feature)
	createDatasource(feature, "remote")
	createProvider(feature, feature)
	createPage(feature, feature)

	rootFile := "lib/injection_container.dart"
	if _, err := os.Stat(rootFile); os.IsNotExist(err) {
		_ = os.WriteFile(rootFile, []byte("// Dependency injection setup\n"), 0644)
		fmt.Println("📝 Created lib/injection_container.dart")
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
	content := fmt.Sprintf(`import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../providers/%s_provider.dart';

class %sPage extends ConsumerWidget {
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(%sProvider);
    return Scaffold(
      appBar: AppBar(title: Text('%s')),
      body: Center(
        child: state.isLoading
          ? CircularProgressIndicator()
          : Text('%s Page'),
      ),
    );
  }
}
`, pageName, pascalCase(pageName), pageName, pageName, pageName)

	writeFile(file, content)
	writeTest(feature, pageName+"_page_test.dart", "// TODO: write page widget tests\n")
}

func writeFile(path, content string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		_ = os.WriteFile(path, []byte(content), 0644)
		fmt.Printf("📝 Created %s\n", path)
	} else {
		fmt.Printf("⚠️ File %s already exists\n", path)
	}
}

func writeTest(feature, filename, content string) {
	dir := filepath.Join("test", "features", feature)
	_ = os.MkdirAll(dir, os.ModePerm)
	path := filepath.Join(dir, filename)

	if _, err := os.Stat(path); os.IsNotExist(err) {
		_ = os.WriteFile(path, []byte(content), 0644)
		fmt.Printf("🧪 Created test %s\n", path)
	}
}

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage:")
		fmt.Println("  new feature <name>")
		fmt.Println("  new page <feature> <pageName>")
		fmt.Println("  new provider <feature> <providerName>")
		fmt.Println("  new entity <feature> <entityName>")
		fmt.Println("  new usecase <feature> <usecaseName>")
		fmt.Println("  new repository <feature> <repoName>")
		fmt.Println("  new datasource <feature> <dsName>")
		return
	}

	cmd := os.Args[1]
	switch cmd {
	case "new":
		subCmd := os.Args[2]
		switch subCmd {
		case "feature":
			createFeature(strings.ToLower(os.Args[3]))
		case "page":
			createPage(strings.ToLower(os.Args[3]), strings.ToLower(os.Args[4]))
		case "provider":
			createProvider(strings.ToLower(os.Args[3]), strings.ToLower(os.Args[4]))
		case "entity":
			createEntity(strings.ToLower(os.Args[3]), strings.ToLower(os.Args[4]))
		case "usecase":
			createUsecase(strings.ToLower(os.Args[3]), strings.ToLower(os.Args[4]))
		case "repository":
			createRepository(strings.ToLower(os.Args[3]), strings.ToLower(os.Args[4]))
		case "datasource":
			createDatasource(strings.ToLower(os.Args[3]), strings.ToLower(os.Args[4]))
		default:
			fmt.Println("❌ Unknown subcommand. Use: feature | page | provider | entity | usecase | repository | datasource")
		}
	default:
		fmt.Println("❌ Unknown command. Use: new")
	}
}