package repository_test

import (
	"path/filepath"
	"testing"

	"github.com/AlbertoBarrago/serval/internal/impact"
	"github.com/AlbertoBarrago/serval/internal/repository"
)

func TestScanAndComputeImpact_SimpleTS(t *testing.T) {
	root, err := filepath.Abs("../../testdata/fixtures/simple-ts")
	if err != nil {
		t.Fatal(err)
	}

	scanner, err := repository.NewScanner(root)
	if err != nil {
		t.Fatalf("NewScanner: %v", err)
	}

	g, err := scanner.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	target := filepath.Join(root, "src/auth/token.ts")
	if !g.HasNode(target) {
		t.Fatalf("expected graph to contain target %s", target)
	}

	result := impact.Compute(g, target)

	wantDirect := filepath.Join(root, "src/auth/middleware.ts")
	if !contains(result.Direct, wantDirect) {
		t.Errorf("expected direct impact to contain %s, got %v", wantDirect, result.Direct)
	}

	wantIndirect := filepath.Join(root, "src/api/client.ts")
	if !contains(result.Indirect, wantIndirect) {
		t.Errorf("expected indirect impact to contain %s, got %v", wantIndirect, result.Indirect)
	}

	unrelated := filepath.Join(root, "src/unrelated.ts")
	if contains(result.Direct, unrelated) || contains(result.Indirect, unrelated) {
		t.Errorf("expected unrelated.ts to not be part of impact")
	}
}

func TestScanAndComputeImpact_SimpleGo(t *testing.T) {
	root, err := filepath.Abs("../../testdata/fixtures/simple-go")
	if err != nil {
		t.Fatal(err)
	}

	scanner, err := repository.NewScanner(root)
	if err != nil {
		t.Fatalf("NewScanner: %v", err)
	}

	g, err := scanner.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	target := filepath.Join(root, "internal/auth/token.go")
	if !g.HasNode(target) {
		t.Fatalf("expected graph to contain target %s", target)
	}

	result := impact.Compute(g, target)

	wantDirect := filepath.Join(root, "main.go")
	if !contains(result.Direct, wantDirect) {
		t.Errorf("expected direct impact to contain %s, got %v", wantDirect, result.Direct)
	}
}

func TestScanAndComputeImpact_SimplePython(t *testing.T) {
	root, err := filepath.Abs("../../testdata/fixtures/simple-python")
	if err != nil {
		t.Fatal(err)
	}

	scanner, err := repository.NewScanner(root)
	if err != nil {
		t.Fatalf("NewScanner: %v", err)
	}

	g, err := scanner.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	target := filepath.Join(root, "src/auth/token.py")
	if !g.HasNode(target) {
		t.Fatalf("expected graph to contain target %s", target)
	}

	result := impact.Compute(g, target)

	wantDirect := filepath.Join(root, "src/auth/middleware.py")
	if !contains(result.Direct, wantDirect) {
		t.Errorf("expected direct impact to contain %s, got %v", wantDirect, result.Direct)
	}

	wantIndirect := filepath.Join(root, "src/api/client.py")
	if !contains(result.Indirect, wantIndirect) {
		t.Errorf("expected indirect impact to contain %s, got %v", wantIndirect, result.Indirect)
	}

	unrelated := filepath.Join(root, "src/unrelated.py")
	if contains(result.Direct, unrelated) || contains(result.Indirect, unrelated) {
		t.Errorf("expected unrelated.py to not be part of impact")
	}
}

func TestScanAndComputeImpact_SimpleJava(t *testing.T) {
	root, err := filepath.Abs("../../testdata/fixtures/simple-java")
	if err != nil {
		t.Fatal(err)
	}

	scanner, err := repository.NewScanner(root)
	if err != nil {
		t.Fatalf("NewScanner: %v", err)
	}

	g, err := scanner.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	target := filepath.Join(root, "src/main/java/com/example/auth/Token.java")
	if !g.HasNode(target) {
		t.Fatalf("expected graph to contain target %s", target)
	}

	result := impact.Compute(g, target)

	wantDirect := filepath.Join(root, "src/main/java/com/example/middleware/Middleware.java")
	if !contains(result.Direct, wantDirect) {
		t.Errorf("expected direct impact to contain %s, got %v", wantDirect, result.Direct)
	}

	wantIndirect := filepath.Join(root, "src/main/java/com/example/api/Client.java")
	if !contains(result.Indirect, wantIndirect) {
		t.Errorf("expected indirect impact to contain %s, got %v", wantIndirect, result.Indirect)
	}

	unrelated := filepath.Join(root, "src/main/java/com/example/Unrelated.java")
	if contains(result.Direct, unrelated) || contains(result.Indirect, unrelated) {
		t.Errorf("expected Unrelated.java to not be part of impact")
	}
}

func TestScanAndComputeImpact_SimpleC(t *testing.T) {
	root, err := filepath.Abs("../../testdata/fixtures/simple-c")
	if err != nil {
		t.Fatal(err)
	}

	scanner, err := repository.NewScanner(root)
	if err != nil {
		t.Fatalf("NewScanner: %v", err)
	}

	g, err := scanner.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	target := filepath.Join(root, "src/auth/token.h")
	if !g.HasNode(target) {
		t.Fatalf("expected graph to contain target %s", target)
	}

	result := impact.Compute(g, target)

	wantDirect := filepath.Join(root, "src/auth/middleware.h")
	if !contains(result.Direct, wantDirect) {
		t.Errorf("expected direct impact to contain %s, got %v", wantDirect, result.Direct)
	}

	wantIndirect := filepath.Join(root, "src/api/client.c")
	if !contains(result.Indirect, wantIndirect) {
		t.Errorf("expected indirect impact to contain %s, got %v", wantIndirect, result.Indirect)
	}

	unrelated := filepath.Join(root, "src/unrelated.h")
	if contains(result.Direct, unrelated) || contains(result.Indirect, unrelated) {
		t.Errorf("expected unrelated.h to not be part of impact")
	}
}

func contains(list []string, item string) bool {
	for _, v := range list {
		if v == item {
			return true
		}
	}
	return false
}

func TestScanAndComputeImpact_RustWorkspace(t *testing.T) {
	root, err := filepath.Abs("../../testdata/fixtures/rust-workspace")
	if err != nil {
		t.Fatal(err)
	}

	scanner, err := repository.NewScanner(root)
	if err != nil {
		t.Fatalf("NewScanner: %v", err)
	}
	g, err := scanner.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	result := impact.Compute(g, filepath.Join(root, "ws/crates/core-types/src/model.rs"))

	// net-io's client uses core_types::model, the CLI uses it too.
	for _, rel := range []string{"ws/crates/core-types/src/lib.rs", "ws/crates/net-io/src/client.rs", "ws/tools/cli/src/main.rs"} {
		if want := filepath.Join(root, rel); !contains(result.Direct, want) {
			t.Errorf("expected direct impact to contain %s, got %v", want, result.Direct)
		}
	}
	// app's service uses core_types::Root (lib.rs) and transport::client.
	if want := filepath.Join(root, "ws/crates/app/src/service.rs"); !contains(result.Indirect, want) {
		t.Errorf("expected indirect impact to contain %s, got %v", want, result.Indirect)
	}
	// legacy is excluded from the workspace: its import stays external.
	if legacy := filepath.Join(root, "ws/crates/legacy/src/lib.rs"); contains(result.Direct, legacy) || contains(result.Indirect, legacy) {
		t.Errorf("expected the excluded crate %s to be outside the impact", legacy)
	}
}

func TestScanAndComputeImpact_SimpleRust(t *testing.T) {
	root, err := filepath.Abs("../../testdata/fixtures/simple-rust")
	if err != nil {
		t.Fatal(err)
	}

	scanner, err := repository.NewScanner(root)
	if err != nil {
		t.Fatalf("NewScanner: %v", err)
	}

	g, err := scanner.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	target := filepath.Join(root, "src/auth/token.rs")
	if !g.HasNode(target) {
		t.Fatalf("expected graph to contain target %s", target)
	}

	generated := filepath.Join(root, "target/debug/generated.rs")
	if g.HasNode(generated) {
		t.Errorf("expected target/ (Cargo build output) to be excluded from the scan")
	}

	result := impact.Compute(g, target)

	// middleware.rs uses it (use super::token), auth/mod.rs declares it.
	for _, rel := range []string{"src/auth/middleware.rs", "src/auth/mod.rs"} {
		if want := filepath.Join(root, rel); !contains(result.Direct, want) {
			t.Errorf("expected direct impact to contain %s, got %v", want, result.Direct)
		}
	}

	wantIndirect := filepath.Join(root, "src/api/client.rs")
	if !contains(result.Indirect, wantIndirect) {
		t.Errorf("expected indirect impact to contain %s, got %v", wantIndirect, result.Indirect)
	}

	unrelated := filepath.Join(root, "src/unrelated.rs")
	if contains(result.Direct, unrelated) || contains(result.Indirect, unrelated) {
		t.Errorf("expected unrelated.rs to not be part of impact")
	}
}
