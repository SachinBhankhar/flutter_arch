package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withTempDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir temp failed: %v", err)
	}
	return tmp
}

func runMain(t *testing.T, args ...string) string {
	t.Helper()
	oldArgs := os.Args
	oldStdout := os.Stdout

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe failed: %v", err)
	}

	os.Stdout = w
	os.Args = append([]string{"flutter_arch"}, args...)

	done := make(chan string, 1)
	go func() {
		var b bytes.Buffer
		_, _ = io.Copy(&b, r)
		done <- b.String()
	}()

	main()

	_ = w.Close()
	out := <-done
	_ = r.Close()

	os.Stdout = oldStdout
	os.Args = oldArgs
	return out
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s failed: %v", path, err)
	}
	return string(data)
}

func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected path to exist: %s (%v)", path, err)
	}
}

func TestCommandUsageAndValidation(t *testing.T) {
	withTempDir(t)

	tests := []struct {
		name   string
		args   []string
		expect string
	}{
		{name: "usage", args: nil, expect: "Usage:"},
		{name: "missing new args", args: []string{"new", "page"}, expect: "missing arguments for 'new'"},
		{name: "missing page args", args: []string{"new", "page", "orders"}, expect: "new page requires <feature> <pageName>"},
		{name: "missing provider args", args: []string{"new", "provider", "orders"}, expect: "new provider requires <feature> <providerName>"},
		{name: "missing entity args", args: []string{"new", "entity", "orders"}, expect: "new entity requires <feature> <entityName>"},
		{name: "missing usecase args", args: []string{"new", "usecase", "orders"}, expect: "new usecase requires <feature> <usecaseName>"},
		{name: "missing repository args", args: []string{"new", "repository", "orders"}, expect: "new repository requires <feature> <repoName>"},
		{name: "missing datasource args", args: []string{"new", "datasource", "orders"}, expect: "new datasource requires <feature> <dsName>"},
		{name: "unknown new subcommand", args: []string{"new", "unknown", "x"}, expect: "Unknown subcommand"},
		{name: "missing migrate args", args: []string{"migrate"}, expect: "missing arguments for 'migrate'"},
		{name: "unknown migrate subcommand", args: []string{"migrate", "unknown"}, expect: "Unknown migrate subcommand"},
		{name: "missing release args", args: []string{"release"}, expect: "missing arguments for 'release'"},
		{name: "unknown release subcommand", args: []string{"release", "unknown"}, expect: "Unknown release subcommand"},
		{name: "unknown root command", args: []string{"oops", "x"}, expect: "Unknown command"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := runMain(t, tt.args...)
			if !strings.Contains(out, tt.expect) {
				t.Fatalf("expected output to contain %q, got:\n%s", tt.expect, out)
			}
		})
	}
}

func TestNewFeatureCreatesScaffold(t *testing.T) {
	withTempDir(t)
	_ = runMain(t, "new", "feature", "orders")

	mustExist(t, filepath.Join("lib", "features", "orders", "domain", "entities", "orders.dart"))
	mustExist(t, filepath.Join("lib", "features", "orders", "domain", "usecases", "example.dart"))
	mustExist(t, filepath.Join("lib", "features", "orders", "domain", "repositories", "orders_repository.dart"))
	mustExist(t, filepath.Join("lib", "features", "orders", "data", "repositories", "orders_repository_impl.dart"))
	mustExist(t, filepath.Join("lib", "features", "orders", "data", "datasources", "remote_datasource.dart"))
	mustExist(t, filepath.Join("lib", "features", "orders", "presentation", "providers", "orders_provider.dart"))
	mustExist(t, filepath.Join("lib", "features", "orders", "presentation", "pages", "orders_page.dart"))
	mustExist(t, filepath.Join("lib", "core", "router.dart"))
	mustExist(t, filepath.Join("lib", "core", "page_names.dart"))
	mustExist(t, filepath.Join("lib", "injection_container.dart"))

	router := mustReadFile(t, filepath.Join("lib", "core", "router.dart"))
	if !strings.Contains(router, "@riverpod") {
		t.Fatalf("expected riverpod router annotation in router.dart, got:\n%s", router)
	}
	if !strings.Contains(router, "Raw<GoRouter> router(Ref ref) => GoRouter(") {
		t.Fatalf("expected Raw<GoRouter> router signature, got:\n%s", router)
	}
	if !strings.Contains(router, "part 'router.g.dart';") {
		t.Fatalf("expected router.g.dart part directive, got:\n%s", router)
	}
}

func TestNewProviderPartFileAndName(t *testing.T) {
	withTempDir(t)
	_ = runMain(t, "new", "provider", "orders", "user_profile")

	content := mustReadFile(t, filepath.Join("lib", "features", "orders", "presentation", "providers", "user_profile_provider.dart"))
	if !strings.Contains(content, "part 'user_profile_provider.g.dart';") {
		t.Fatalf("provider file has wrong part name:\n%s", content)
	}
	if !strings.Contains(content, "int userProfile(Ref ref) => 0;") {
		t.Fatalf("provider file has wrong function name:\n%s", content)
	}
}

func TestNewPageUsesFallbackProviderAndCamelCaseWatch(t *testing.T) {
	withTempDir(t)
	_ = runMain(t, "new", "page", "checkout", "payment")

	pagePath := filepath.Join("lib", "features", "checkout", "presentation", "pages", "payment_page.dart")
	content := mustReadFile(t, pagePath)

	if !strings.Contains(content, "import '../providers/checkout_provider.dart';") {
		t.Fatalf("page should import feature fallback provider:\n%s", content)
	}
	if !strings.Contains(content, "ref.watch(checkoutProvider);") {
		t.Fatalf("page should watch camelCase provider variable:\n%s", content)
	}

	mustExist(t, filepath.Join("lib", "features", "checkout", "presentation", "providers", "checkout_provider.dart"))
}

func TestNewCommandsCreateExpectedFilesAndTests(t *testing.T) {
	withTempDir(t)

	_ = runMain(t, "new", "entity", "orders", "invoice")
	_ = runMain(t, "new", "usecase", "orders", "fetch_orders")
	_ = runMain(t, "new", "repository", "orders", "orders")
	_ = runMain(t, "new", "datasource", "orders", "remote")
	_ = runMain(t, "new", "provider", "orders", "orders")
	_ = runMain(t, "new", "page", "orders", "details")

	mustExist(t, filepath.Join("lib", "features", "orders", "domain", "entities", "invoice.dart"))
	mustExist(t, filepath.Join("lib", "features", "orders", "domain", "usecases", "fetch_orders.dart"))
	mustExist(t, filepath.Join("lib", "features", "orders", "domain", "repositories", "orders_repository.dart"))
	mustExist(t, filepath.Join("lib", "features", "orders", "data", "repositories", "orders_repository_impl.dart"))
	mustExist(t, filepath.Join("lib", "features", "orders", "data", "datasources", "remote_datasource.dart"))
	mustExist(t, filepath.Join("lib", "features", "orders", "presentation", "providers", "orders_provider.dart"))
	mustExist(t, filepath.Join("lib", "features", "orders", "presentation", "pages", "details_page.dart"))

	mustExist(t, filepath.Join("test", "features", "orders", "domain", "entities", "invoice_entity_test.dart"))
	mustExist(t, filepath.Join("test", "features", "orders", "domain", "usecases", "fetch_orders_usecase_test.dart"))
	mustExist(t, filepath.Join("test", "features", "orders", "data", "repositories", "orders_repository_test.dart"))
	mustExist(t, filepath.Join("test", "features", "orders", "data", "datasources", "remote_datasource_test.dart"))
	mustExist(t, filepath.Join("test", "features", "orders", "presentation", "providers", "orders_provider_test.dart"))
	mustExist(t, filepath.Join("test", "features", "orders", "presentation", "pages", "details_page_test.dart"))
}

func TestPageCommandNoDuplicateRouteOrConstant(t *testing.T) {
	withTempDir(t)
	_ = runMain(t, "new", "page", "orders", "details")
	_ = runMain(t, "new", "page", "orders", "details")

	router := mustReadFile(t, filepath.Join("lib", "core", "router.dart"))
	if strings.Count(router, "GoRoute(path: kDetailsPage, builder: (context, state) => const DetailsPage()),") != 1 {
		t.Fatalf("route should be added once, got:\n%s", router)
	}

	names := mustReadFile(t, filepath.Join("lib", "core", "page_names.dart"))
	if strings.Count(names, "const kDetailsPage = '/details';") != 1 {
		t.Fatalf("page constant should be added once, got:\n%s", names)
	}
}

func TestMigrateTestsMovesLegacyFlatFiles(t *testing.T) {
	withTempDir(t)

	featureDir := filepath.Join("test", "features", "orders")
	if err := os.MkdirAll(featureDir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	legacy := map[string]string{
		"invoice_entity_test.dart":    "domain/entities",
		"fetch_usecase_test.dart":     "domain/usecases",
		"orders_repository_test.dart": "data/repositories",
		"remote_datasource_test.dart": "data/datasources",
		"orders_provider_test.dart":   "presentation/providers",
		"details_page_test.dart":      "presentation/pages",
		"misc_test.dart":              "",
	}

	for name := range legacy {
		path := filepath.Join(featureDir, name)
		if err := os.WriteFile(path, []byte("// legacy"), 0644); err != nil {
			t.Fatalf("write legacy file failed: %v", err)
		}
	}

	_ = runMain(t, "migrate", "tests")

	for name, rel := range legacy {
		src := filepath.Join(featureDir, name)
		if rel == "" {
			mustExist(t, src)
			continue
		}

		dst := filepath.Join(featureDir, rel, name)
		mustExist(t, dst)
		if _, err := os.Stat(src); !os.IsNotExist(err) {
			t.Fatalf("expected source to be moved: %s", src)
		}
	}
}

func TestNormalizeImportPartOrderMovesImportsAbovePart(t *testing.T) {
	input := "part 'router.g.dart';\n\nimport '../features/shared/presentation/pages/video_player_test_page.dart';\n// AUTO_IMPORTS\n"
	got := normalizeImportPartOrder(input)

	importIdx := strings.Index(got, "import '../features/shared/presentation/pages/video_player_test_page.dart';")
	partIdx := strings.Index(got, "part 'router.g.dart';")
	if importIdx == -1 || partIdx == -1 {
		t.Fatalf("expected both import and part to exist, got:\n%s", got)
	}
	if importIdx > partIdx {
		t.Fatalf("expected import before part, got:\n%s", got)
	}
}
