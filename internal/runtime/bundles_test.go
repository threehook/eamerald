//nolint:testpackage
package runtime

import (
	"strings"
	"testing"
)

// Package roots of two policies that could be bundled together.
const (
	rootAuthz     = "authz"
	rootLaadpalen = "laadpalen"
)

// TestPolicyRoots covers the reduction the fallback selection runs on: the
// roots are what opa.policy_root is matched against, so a bundle of
// per-doelbinding packages has to collapse to the one root they share.
func TestPolicyRoots(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		packages []string
		want     []string
	}{
		{
			name:     "a top-level package is its own root",
			packages: []string{rootAuthz},
			want:     []string{rootAuthz},
		},
		{
			name:     "packages sharing a root collapse onto it",
			packages: []string{"doelbinding." + rootLaadpalen, "doelbinding.subsidies"},
			want:     []string{"doelbinding"},
		},
		{
			name:     "distinct roots are all kept, in input order",
			packages: []string{rootAuthz, "doelbinding.subsidies", "lib.postcode"},
			want:     []string{rootAuthz, "doelbinding", "lib"},
		},
		{
			name:     "nothing loaded yields no roots",
			packages: []string{},
			want:     []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := PolicyRoots(tc.packages)

			if len(got) != len(tc.want) {
				t.Fatalf("PolicyRoots(%v) = %v, want %v", tc.packages, got, tc.want)
			}

			for i, want := range tc.want {
				if got[i] != want {
					t.Errorf("PolicyRoots(%v)[%d] = %q, want %q", tc.packages, i, got[i], want)
				}
			}
		})
	}
}

// TestSelectPolicyRoot covers which policy an instance serves when a request
// selects none of its own. That has to be decided by the deployment - and a
// bundle with several package roots must not resolve to whichever one the
// policy store happens to list first.
func TestSelectPolicyRoot(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		configured string
		roots      []string
		want       string
		wantErr    string
	}{
		{
			name:  "a single loaded policy needs no configuration",
			roots: []string{rootAuthz},
			want:  rootAuthz,
		},
		{
			name:       "configuration picks one of several policies",
			configured: rootLaadpalen,
			roots:      []string{rootAuthz, rootLaadpalen},
			want:       rootLaadpalen,
		},
		{
			name:       "configuration wins over a single loaded policy",
			configured: rootAuthz,
			roots:      []string{rootAuthz},
			want:       rootAuthz,
		},
		{
			name:    "several policies without configuration is ambiguous",
			roots:   []string{rootAuthz, rootLaadpalen},
			wantErr: "set opa.policy_root",
		},
		{
			name:       "a configured policy that is not loaded is an error",
			configured: "hr",
			roots:      []string{rootAuthz, rootLaadpalen},
			wantErr:    "is not loaded",
		},
		{
			name:    "no policy loaded at all is an error",
			roots:   []string{},
			wantErr: "no policy loaded",
		},
		{
			name:       "a configured policy is still checked when nothing is loaded",
			configured: rootAuthz,
			roots:      []string{},
			wantErr:    "no policy loaded",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rt := &Runtime{Config: &Config{PolicyRoot: tc.configured}}

			got, err := rt.SelectPolicyRoot(tc.roots)

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("SelectPolicyRoot(%v) = %q, want an error", tc.roots, got)
				}

				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("SelectPolicyRoot(%v) error = %q, want it to mention %q", tc.roots, err, tc.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("SelectPolicyRoot(%v): %v", tc.roots, err)
			}

			if got != tc.want {
				t.Errorf("SelectPolicyRoot(%v) = %q, want %q", tc.roots, got, tc.want)
			}
		})
	}
}
