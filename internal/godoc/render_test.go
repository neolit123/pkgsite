// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package godoc

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/safehtml/template"
	"golang.org/x/net/html"
	"golang.org/x/pkgsite/internal"
	"golang.org/x/pkgsite/internal/experiment"
	"golang.org/x/pkgsite/internal/godoc/dochtml"
	"golang.org/x/pkgsite/internal/source"
	"golang.org/x/pkgsite/internal/testing/htmlcheck"
)

var templateSource = template.TrustedSourceFromConstant("../../content/static/html/doc")

var (
	in      = htmlcheck.In
	hasText = htmlcheck.HasText
)

func TestRender(t *testing.T) {
	dochtml.LoadTemplates(templateSource)
	ctx := context.Background()
	si := source.NewGitHubInfo("a.com/M", "", "abcde")
	mi := &ModuleInfo{
		ModulePath:      "a.com/M",
		ResolvedVersion: "v1.2.3",
		ModulePackages:  nil,
	}

	// Use a Package created locally and without nodes removed as the desired doc.
	p, err := packageForDir(filepath.Join("testdata", "p"), false)
	if err != nil {
		t.Fatal(err)
	}

	wantSyn, wantImports, wantDoc, _, err := p.Render(ctx, "p", si, mi)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(wantDoc.String(), "return") {
		t.Fatal("doc rendered with function bodies")
	}

	check := func(p *Package) {
		t.Helper()
		gotSyn, gotImports, gotDoc, _, err := p.Render(ctx, "p", si, mi)
		if err != nil {
			t.Fatal(err)
		}
		if gotSyn != wantSyn {
			t.Errorf("synopsis: got %q, want %q", gotSyn, wantSyn)
		}
		if !cmp.Equal(gotImports, wantImports) {
			t.Errorf("imports: got %v, want %v", gotImports, wantImports)
		}
		if diff := cmp.Diff(wantDoc.String(), gotDoc.String()); diff != "" {
			t.Errorf("doc mismatch (-want, +got):\n%s", diff)
			t.Logf("---- want ----\n%s", wantDoc)
			t.Logf("---- got ----\n%s", gotDoc)
		}
	}

	// Verify that removing AST nodes doesn't change the doc.
	p, err = packageForDir(filepath.Join("testdata", "p"), true)
	if err != nil {
		t.Fatal(err)
	}
	check(p)

	// Verify that encoding then decoding generates the same doc.
	// We can't re-use p to encode because it's been rendered.
	p, err = packageForDir(filepath.Join("testdata", "p"), true)
	if err != nil {
		t.Fatal(err)
	}
	bytes, err := p.Encode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	p2, err := DecodePackage(bytes)
	if err != nil {
		t.Fatal(err)
	}
	check(p2)
}

func TestRenderParts_SinceVersion(t *testing.T) {
	dochtml.LoadTemplates(templateSource)
	ctx := experiment.NewContext(context.Background(),
		internal.ExperimentSymbolHistoryMainPage)
	si := source.NewGitHubInfo("a.com/M", "", "abcde")
	mi := &ModuleInfo{
		ModulePath:      "a.com/M",
		ResolvedVersion: "v1.2.3",
		ModulePackages:  nil,
	}

	// Use a Package created locally and without nodes removed as the desired doc.
	p, err := packageForDir(filepath.Join("testdata", "p"), false)
	if err != nil {
		t.Fatal(err)
	}

	nameToVersion := map[string]string{
		// F is a function.  The since version won't appear on the main page,
		// because F was introduced at the earliest version.
		"F": "v1.0.0",
		// I is a type.
		"I": "v1.3.0",
		// T is a type.
		"T": "v1.3.0",
		// TF is a type function.
		"TF": "v1.4.0",
		// TF is a method.
		"T.M": "v1.4.0",
	}
	parts, err := p.RenderParts(ctx, "p", si, mi, nameToVersion)
	if err != nil {
		t.Fatal(err)
	}

	htmlDoc, err := html.Parse(strings.NewReader(parts.Body.String()))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		symbol, class string
		wantEmpty     bool
	}{
		{"F", ".Documentation-functionHeader", true},
		{"I", ".Documentation-typeHeader", false},
		{"T", "h4#T", false},
		{"TF", ".Documentation-typeFuncHeader", false},
		{"T.M", ".Documentation-typeMethodHeader", false},
	} {
		t.Run(test.class, func(t *testing.T) {
			if test.wantEmpty {
				checker := in(test.class,
					in(".Documentation-sinceVersion", hasText("")))
				if err := checker(htmlDoc); err != nil {
					t.Error(err)
				}
			} else {
				checker := in(test.class,
					in(".Documentation-sinceVersion",
						in(".Documentation-sinceVersionVersion",
							hasText(nameToVersion[test.symbol]))))
				if err := checker(htmlDoc); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
