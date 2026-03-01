# Setup For Future Agents

This repository is a Go CLI that scaffolds Flutter clean-architecture files.

## 1) Go test setup

Use a writable Go cache path in this environment:

```bash
GOCACHE=/tmp/go-cache go test ./...
```

## 2) Flutter E2E test setup (with FVM)

Do not run `fvm use stable` in repo root unless you are inside a Flutter project.

Create a disposable Flutter app inside this repo:

```bash
fvm spawn stable create __flutter_test_project__2
```

The disposable project path is gitignored by:

```text
__flutter_test_project__*/
```

## 3) Build and run this tool against the Flutter test project

```bash
GOCACHE=/tmp/go-cache go build -o /tmp/flutter_arch_tool .
cd __flutter_test_project__2
/tmp/flutter_arch_tool new feature orders
/tmp/flutter_arch_tool new provider orders user_profile
/tmp/flutter_arch_tool new page orders checkout
```

## 4) Use `fvm flutter` commands in the Flutter test project

```bash
fvm use stable --force
fvm flutter pub add go_router flutter_riverpod riverpod_annotation
fvm flutter test
```

## 5) Migration command for old flat tests

If old projects still have tests directly under `test/features/<feature>/`, run:

```bash
/tmp/flutter_arch_tool migrate tests
```

This moves files into the structured folders:

- `data/datasources`
- `data/repositories`
- `domain/entities`
- `domain/usecases`
- `presentation/pages`
- `presentation/providers`
- `presentation/widgets`
