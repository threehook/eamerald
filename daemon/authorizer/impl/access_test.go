//nolint:testpackage
package impl

import (
	"testing"

	"github.com/aserto-dev/go-authorizer/aserto/authorizer/v2/api"
	dsc "github.com/aserto-dev/go-directory/aserto/directory/common/v3"
	dsa "github.com/authzen/access.go/api/access/v1"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	testAction       = "request_laadpaal"
	testSubjectID    = "jerry@example.com"
	testSubjectType  = "user"
	testResourceType = "address"
	testPostcode     = "postcode"
	testReason       = "reason"
	testPolicyRoot   = "authz"
	testTenant       = "gemeente"
	testJWT          = "eyJhbGciOi.not-a-real-one"
	testPostcodeVal  = "1111BB"
	testResourceID   = "1111BB-2"
)

func mustStruct(t *testing.T, fields map[string]any) *structpb.Struct {
	t.Helper()

	s, err := structpb.NewStruct(fields)
	if err != nil {
		t.Fatalf("structpb.NewStruct(%v): %v", fields, err)
	}

	return s
}

// TestFlatten covers the mapping a policy actually sees. The laadpalen
// policy reads input.resource.postcode, so an AuthZEN resource has to arrive
// with its properties at the top level rather than nested under
// "properties" - that is what lets an existing Is()/Query() policy serve the
// Access API unchanged.
func TestFlatten(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		properties *structpb.Struct
		fields     map[string]string
		want       map[string]any
	}{
		{
			name:       "properties are lifted to the top level",
			properties: mustStruct(t, map[string]any{testPostcode: testPostcodeVal, "huisnummer": 2}),
			fields:     map[string]string{typeField: testResourceType},
			want: map[string]any{
				testPostcode: testPostcodeVal, "huisnummer": float64(2), typeField: testResourceType,
			},
		},
		{
			name:       "structural fields are folded in",
			properties: nil,
			fields:     map[string]string{typeField: testSubjectType, idField: testSubjectID},
			want:       map[string]any{typeField: testSubjectType, idField: testSubjectID},
		},
		{
			name:       "empty structural fields are omitted",
			properties: nil,
			fields:     map[string]string{typeField: testSubjectType, idField: ""},
			want:       map[string]any{typeField: testSubjectType},
		},
		{
			name:       "a property of the same name is not overwritten",
			properties: mustStruct(t, map[string]any{typeField: "from-properties"}),
			fields:     map[string]string{typeField: "from-resource"},
			want:       map[string]any{typeField: "from-properties"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := flatten(tc.properties, tc.fields)

			if len(got) != len(tc.want) {
				t.Fatalf("flatten() = %v, want %v", got, tc.want)
			}

			for k, want := range tc.want {
				if got[k] != want {
					t.Errorf("flatten()[%q] = %v (%T), want %v (%T)", k, got[k], got[k], want, want)
				}
			}
		})
	}
}

func TestIdentityContext(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		subject      *dsa.Subject
		wantType     api.IdentityType
		wantIdentity string
	}{
		{
			name:         "id becomes a subject identity",
			subject:      &dsa.Subject{Type: testSubjectType, Id: testSubjectID},
			wantType:     api.IdentityType_IDENTITY_TYPE_SUB,
			wantIdentity: testSubjectID,
		},
		{
			name: "a jwt property takes precedence over the id",
			subject: &dsa.Subject{
				Type:       testSubjectType,
				Id:         testSubjectID,
				Properties: mustStruct(t, map[string]any{subjectJWTProperty: testJWT}),
			},
			wantType:     api.IdentityType_IDENTITY_TYPE_JWT,
			wantIdentity: testJWT,
		},
		{
			name:     "no subject resolves to no identity",
			subject:  nil,
			wantType: api.IdentityType_IDENTITY_TYPE_NONE,
		},
		{
			name:     "a subject without an id resolves to no identity",
			subject:  &dsa.Subject{Type: testSubjectType},
			wantType: api.IdentityType_IDENTITY_TYPE_NONE,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := identityContext(tc.subject)

			if got.GetType() != tc.wantType {
				t.Errorf("type = %v, want %v", got.GetType(), tc.wantType)
			}

			if got.GetIdentity() != tc.wantIdentity {
				t.Errorf("identity = %q, want %q", got.GetIdentity(), tc.wantIdentity)
			}
		})
	}
}

func TestEvaluationResponse(t *testing.T) {
	t.Parallel()

	const (
		granted = "Toegekend"
		denied  = "Reeds laadpaal aanwezig"
	)

	cases := []struct {
		name        string
		binding     any
		wantErr     bool
		wantDecided bool
		wantReason  string
	}{
		{
			name:        "an authzen decision object maps through verbatim",
			binding:     map[string]any{decisionField: true, contextField: map[string]any{testReason: granted}},
			wantDecided: true,
			wantReason:  granted,
		},
		{
			name:        "a denial keeps its context",
			binding:     map[string]any{decisionField: false, contextField: map[string]any{testReason: denied}},
			wantDecided: false,
			wantReason:  denied,
		},
		{
			name:        "a decision object without a context is still a decision",
			binding:     map[string]any{decisionField: true},
			wantDecided: true,
		},
		{
			name:        "a bare boolean is a decision without a context",
			binding:     true,
			wantDecided: true,
		},
		{
			name:    "an object without a decision field is rejected",
			binding: map[string]any{contextField: map[string]any{testReason: granted}},
			wantErr: true,
		},
		{
			name:    "a non-boolean decision field is rejected",
			binding: map[string]any{decisionField: "yes"},
			wantErr: true,
		},
		{
			name:    "an unrelated result shape is rejected",
			binding: []any{1, 2, 3},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resp, err := evaluationResponse(testAction, tc.binding)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("evaluationResponse() = %v, want an error", resp)
				}

				return
			}

			if err != nil {
				t.Fatalf("evaluationResponse(): %v", err)
			}

			if resp.GetDecision() != tc.wantDecided {
				t.Errorf("decision = %v, want %v", resp.GetDecision(), tc.wantDecided)
			}

			if got := resp.GetContext().GetFields()[testReason].GetStringValue(); got != tc.wantReason {
				t.Errorf("context.reason = %q, want %q", got, tc.wantReason)
			}
		})
	}
}

func TestDefaulted(t *testing.T) {
	t.Parallel()

	batch := &dsa.EvaluationsRequest{
		Subject:  &dsa.Subject{Type: testSubjectType, Id: testSubjectID},
		Action:   &dsa.Action{Name: testAction},
		Resource: &dsa.Resource{Type: testResourceType, Id: "1111AA-1"},
	}

	t.Run("unset fields fall back to the batch defaults", func(t *testing.T) {
		t.Parallel()

		got := defaulted(batch, &dsa.EvaluationRequest{})

		if got.GetSubject().GetId() != testSubjectID {
			t.Errorf("subject id = %q, want %q", got.GetSubject().GetId(), testSubjectID)
		}

		if got.GetAction().GetName() != testAction {
			t.Errorf("action = %q, want %q", got.GetAction().GetName(), testAction)
		}

		if got.GetResource().GetId() != "1111AA-1" {
			t.Errorf("resource id = %q, want %q", got.GetResource().GetId(), "1111AA-1")
		}
	})

	t.Run("set fields override the batch defaults", func(t *testing.T) {
		t.Parallel()

		got := defaulted(batch, &dsa.EvaluationRequest{
			Resource: &dsa.Resource{Type: testResourceType, Id: testResourceID},
		})

		if got.GetResource().GetId() != testResourceID {
			t.Errorf("resource id = %q, want %q", got.GetResource().GetId(), testResourceID)
		}

		if got.GetSubject().GetId() != testSubjectID {
			t.Errorf("subject id = %q, want %q", got.GetSubject().GetId(), testSubjectID)
		}
	})
}

func TestWithPolicyPath(t *testing.T) {
	t.Parallel()

	t.Run("the deciding policy is recorded alongside the caller's context", func(t *testing.T) {
		t.Parallel()

		got := withPolicyPath(mustStruct(t, map[string]any{"tenant": testTenant}), testPolicyRoot)

		if p := got.GetFields()[policyPathKey].GetStringValue(); p != testPolicyRoot {
			t.Errorf("%s = %q, want %q", policyPathKey, p, testPolicyRoot)
		}

		if tenant := got.GetFields()["tenant"].GetStringValue(); tenant != testTenant {
			t.Errorf("caller context was dropped: tenant = %q", tenant)
		}
	})

	t.Run("an unresolved policy leaves the context untouched", func(t *testing.T) {
		t.Parallel()

		base := mustStruct(t, map[string]any{"tenant": testTenant})

		if got := withPolicyPath(base, ""); got != base {
			t.Errorf("withPolicyPath() = %v, want the base context unchanged", got)
		}
	})
}

// TestEvalMetaRequest is the data-minimisation guard: whatever a caller sends
// as subject properties, the decision record carries only what the identity
// resolved to. A bearer token passed in must never reach the log.
func TestEvalMetaRequest(t *testing.T) {
	t.Parallel()

	req := &dsa.EvaluationRequest{
		Subject: &dsa.Subject{
			Type:       testSubjectType,
			Id:         testSubjectID,
			Properties: mustStruct(t, map[string]any{subjectJWTProperty: testJWT}),
		},
		Action: &dsa.Action{Name: testAction},
		Resource: &dsa.Resource{
			Type:       testResourceType,
			Properties: mustStruct(t, map[string]any{testPostcode: testPostcodeVal}),
		},
	}

	meta := evalMeta{
		policyRoot: testPolicyRoot,
		identity:   identityContext(req.GetSubject()),
		user:       &dsc.Object{Type: testSubjectType, Id: testSubjectID},
	}

	logged := meta.request(req)

	if logged.GetSubject().GetProperties() != nil {
		t.Errorf("subject properties reached the decision log: %v", logged.GetSubject().GetProperties())
	}

	if id := logged.GetSubject().GetId(); id != testSubjectID {
		t.Errorf("subject id = %q, want the resolved user %q", id, testSubjectID)
	}

	if p := logged.GetContext().GetFields()[policyPathKey].GetStringValue(); p != testPolicyRoot {
		t.Errorf("%s = %q, want %q", policyPathKey, p, testPolicyRoot)
	}

	// The resource is the caller's, unmodified: it is the decision's subject
	// matter, not a credential.
	if got := logged.GetResource().GetProperties().GetFields()[testPostcode].GetStringValue(); got != testPostcodeVal {
		t.Errorf("resource postcode = %q, want %q", got, testPostcodeVal)
	}
}

// TestScrubSubject guards the same property leak on the batch sub-requests,
// which carry their own subjects.
func TestScrubSubject(t *testing.T) {
	t.Parallel()

	got := scrubSubject(&dsa.Subject{
		Type:       testSubjectType,
		Id:         testSubjectID,
		Properties: mustStruct(t, map[string]any{subjectJWTProperty: testJWT}),
	})

	if got.GetProperties() != nil {
		t.Errorf("properties = %v, want them dropped", got.GetProperties())
	}

	if got.GetId() != testSubjectID || got.GetType() != testSubjectType {
		t.Errorf("scrubSubject() = %v, want the type and id preserved", got)
	}

	if scrubSubject(nil) != nil {
		t.Error("scrubSubject(nil) should stay nil")
	}
}
