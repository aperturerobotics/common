package protogen

import "testing"

func TestRustModulesRenderNestsPackages(t *testing.T) {
	// Mix a root schema with nested packages and multiple files in one package.
	modules := NewRustModules()
	files := []struct{ pkg, file string }{
		{"bifrost.api", "net/daemon/api/api.pb.rs"},
		{"bifrost.api.controller", "net/daemon/api/controller/controller.pb.rs"},
		{"app", "app/app_srpc.pb.rs"},
		{"app", "app/app.pb.rs"},
		{"", "root.pb.rs"},
	}
	for _, f := range files {
		modules.Add(f.pkg, f.file)
	}

	// Deterministic includes belong under their complete package hierarchy.
	expected := `include!(concat!(env!("CARGO_MANIFEST_DIR"), "/root.pb.rs"));
pub mod app {
    include!(concat!(env!("CARGO_MANIFEST_DIR"), "/app/app.pb.rs"));
    include!(concat!(env!("CARGO_MANIFEST_DIR"), "/app/app_srpc.pb.rs"));
}
pub mod bifrost {
    pub mod api {
        include!(concat!(env!("CARGO_MANIFEST_DIR"), "/net/daemon/api/api.pb.rs"));
        pub mod controller {
            include!(concat!(env!("CARGO_MANIFEST_DIR"), "/net/daemon/api/controller/controller.pb.rs"));
        }
    }
}
`
	if got := modules.Render(); got != expected {
		t.Fatalf("unexpected modules:\n%s\nwant:\n%s", got, expected)
	}
}

func TestRustModulesNamePackagesAsProstDoes(t *testing.T) {
	// Exercise casing, keywords and identifiers that need a leading underscore.
	modules := NewRustModules()
	files := []struct{ pkg, file string }{
		{"Upper.case", "a.pb.rs"},
		{"upper.case", "b.pb.rs"},
		{"a.type", "c.pb.rs"},
		{"net.HTTPServer.v1", "d.pb.rs"},
		{"self.super", "e.pb.rs"},
		{"v1.2go", "f.pb.rs"},
	}
	for _, f := range files {
		modules.Add(f.pkg, f.file)
	}

	// Normalized names must agree with Prost's references between packages.
	expected := `pub mod a {
    pub mod r#type {
        include!(concat!(env!("CARGO_MANIFEST_DIR"), "/c.pb.rs"));
    }
}
pub mod net {
    pub mod http_server {
        pub mod v1 {
            include!(concat!(env!("CARGO_MANIFEST_DIR"), "/d.pb.rs"));
        }
    }
}
pub mod self_ {
    pub mod super_ {
        include!(concat!(env!("CARGO_MANIFEST_DIR"), "/e.pb.rs"));
    }
}
pub mod upper {
    pub mod case {
        include!(concat!(env!("CARGO_MANIFEST_DIR"), "/a.pb.rs"));
        include!(concat!(env!("CARGO_MANIFEST_DIR"), "/b.pb.rs"));
    }
}
pub mod v1 {
    pub mod _2go {
        include!(concat!(env!("CARGO_MANIFEST_DIR"), "/f.pb.rs"));
    }
}
`
	if got := modules.Render(); got != expected {
		t.Fatalf("unexpected modules:\n%s\nwant:\n%s", got, expected)
	}
}

func TestRustModulesQuoteIncludePaths(t *testing.T) {
	modules := NewRustModules()
	modules.Add("", "dir with \"quote\"/back\\slash\tname.pb.rs")

	expected := `include!(concat!(env!("CARGO_MANIFEST_DIR"), "/dir with \"quote\"/back\\slash\tname.pb.rs"));
`
	if got := modules.Render(); got != expected {
		t.Fatalf("unexpected modules:\n%s\nwant:\n%s", got, expected)
	}
}

func TestRustModuleName(t *testing.T) {
	cases := map[string]string{
		"app":        "app",
		"Upper":      "upper",
		"HTTPServer": "http_server",
		"getHTTPUrl": "get_http_url",
		"type":       "r#type",
		"Type":       "r#type",
		"async":      "r#async",
		"self":       "self_",
		"crate":      "crate_",
		"super":      "super_",
		"2go":        "_2go",
		"v1":         "v1",
	}
	for segment, want := range cases {
		if got := rustModuleName(segment); got != want {
			t.Errorf("rustModuleName(%q) = %q, want %q", segment, got, want)
		}
	}
}
