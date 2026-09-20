package common

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	cerr "github.com/aserto-dev/errors"
	dsr "github.com/aserto-dev/go-directory/aserto/directory/reader/v3"
	dsa "github.com/authzen/access.go/api/access/v1"
	"github.com/threehook/eamerald/mrld/cc"
	azc "github.com/threehook/eamerald/mrld/clients/authorizer"
	dsc "github.com/threehook/eamerald/mrld/clients/directory"

	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	skipped string = "SKIPPED"
)

var (
	ErrSkippedAuthorizerAssertion = cerr.NewAsertoError("T10001", codes.Internal, http.StatusInternalServerError, "no authorizer client")
	ErrSkippedDirectoryAssertion  = cerr.NewAsertoError("T10002", codes.Internal, http.StatusInternalServerError, "no directory client")
)

type TestExecCmd struct {
	Files   []string `arg:""  default:"assertions.json" help:"path to assertions file" sep:"none" optional:""`
	Stdin   bool     `flag:"" default:"false" help:"read assertions from --stdin"`
	Summary bool     `flag:"" default:"false" help:"display test summary"`
	Format  string   `flag:"" default:"table" help:"output format (table|csv)" enum:"table,csv"`
	Desc    string   `flag:"" default:"off" enum:"off,on,on-error" help:"output descriptions (off|on|on-error)"`
}

type TestRunner struct {
	cmd      *TestExecCmd
	azClient *azc.Client
	dsClient *dsc.Client
	results  *TestResults
}

func NewDirectoryTestRunner(ctx context.Context, cmd *TestExecCmd, dsConfig *dsc.Config) (*TestRunner, error) {
	dsClient, err := dsc.NewClient(ctx, dsConfig)
	if err != nil {
		return nil, err
	}

	return &TestRunner{
		cmd:      cmd,
		dsClient: dsClient,
	}, nil
}

func NewAuthorizerTestRunner(ctx context.Context, cmd *TestExecCmd, azConfig *azc.Config) (*TestRunner, error) {
	azClient, err := azc.NewClient(ctx, azConfig)
	if err != nil {
		return nil, err
	}

	return &TestRunner{
		cmd:      cmd,
		azClient: azClient,
	}, nil
}

func NewTestRunner(ctx context.Context, cmd *TestExecCmd, azConfig *azc.Config, dsConfig *dsc.Config) (*TestRunner, error) {
	dsClient, err := dsc.NewClient(ctx, dsConfig)
	if err != nil {
		return nil, err
	}

	azClient, err := azc.NewClient(ctx, azConfig)
	if err != nil {
		return nil, err
	}

	return &TestRunner{
		cmd:      cmd,
		azClient: azClient,
		dsClient: dsClient,
	}, nil
}

func (runner *TestRunner) Run(ctx context.Context) error {
	if runner.cmd.Stdin {
		return runner.exec(ctx, os.Stdin)
	}

	for _, file := range runner.cmd.Files {
		if err := runner.execFile(ctx, file); err != nil {
			return err
		}
	}

	return nil
}

func (runner *TestRunner) execFile(ctx context.Context, file string) error {
	r, err := os.Open(file)
	if err != nil {
		return err
	}
	defer r.Close()

	cc.Con().Info().Msg(file)

	return runner.exec(ctx, r)
}

var pbUnmarshal = protojson.UnmarshalOptions{
	AllowPartial:   false,
	DiscardUnknown: true,
	RecursionLimit: 0,
}

//nolint:funlen
func (runner *TestRunner) exec(ctx context.Context, r *os.File) error {
	csvWriter := csv.NewWriter(os.Stdout)

	var assertions struct {
		Assertions []json.RawMessage `json:"assertions"`
	}

	dec := json.NewDecoder(r)
	if err := dec.Decode(&assertions); err != nil {
		return err
	}

	runner.results = NewTestResults(assertions.Assertions)

	for i := range assertions.Assertions {
		var msg structpb.Struct
		if err := pbUnmarshal.Unmarshal(assertions.Assertions[i], &msg); err != nil {
			return err
		}

		expected, ok := GetBool(&msg, Expected)
		if !ok {
			return errors.Errorf("no expected outcome of assertion defined")
		}

		description, _ := GetString(&msg, Description)

		checkType := GetCheckType(&msg)
		if checkType == CheckUnknown {
			return errors.Errorf("unknown check type")
		}

		reqVersion := getReqVersion(msg.GetFields()[CheckTypeMapStr[checkType]])
		if reqVersion == 0 {
			return errors.Errorf("unknown request version")
		}

		result := setCheckType(ctx, checkType, reqVersion, runner, &msg)

		runner.results.Passed(result.Outcome == expected)

		if result.Err != nil {
			switch {
			case errors.Is(result.Err, ErrSkippedAuthorizerAssertion):
				result.Err = nil

				runner.results.IncrSkipped()
			case errors.Is(result.Err, ErrSkippedDirectoryAssertion):
				result.Err = nil

				runner.results.IncrSkipped()
			default:
				runner.results.IncrErrored()
			}
		}

		if runner.cmd.Format == TestOutputCSV {
			if i == 0 {
				result.PrintCSVHeader(csvWriter)
			}

			result.PrintCSV(csvWriter, i, expected, checkType, description)
		}

		if runner.cmd.Format == TestOutputTable {
			result.PrintTable(os.Stdout, i, expected, checkType, cc.NoColor())
			PrintDesc(runner.cmd.Desc, description, result, expected)
		}
	}

	if runner.cmd.Format == TestOutputCSV {
		csvWriter.Flush()
	}

	if runner.cmd.Summary {
		runner.results.PrintSummary(os.Stdout)
	}

	if runner.results.failed != 0 || runner.results.errored != 0 {
		return errors.Errorf("%d tests failed, %d tests errored", runner.results.failed, runner.results.errored)
	}

	return nil
}

func setCheckType(ctx context.Context, checkType CheckType, reqVersion int, runner *TestRunner, msg *structpb.Struct) *CheckResult {
	var result *CheckResult

	switch {
	case checkType == Check && reqVersion == 3:
		result = checkV3(ctx, runner.dsClient, msg.GetFields()[CheckTypeMapStr[checkType]])
	case checkType == Evaluation:
		result = evaluationV1(ctx, runner.dsClient, msg.GetFields()[CheckTypeMapStr[checkType]])
	case checkType == AuthorizerEvaluation:
		result = authorizerEvaluationV1(ctx, runner.azClient, msg.GetFields()[CheckTypeMapStr[checkType]])
	case checkType == Evaluations:
		result = evaluationsV1(ctx, runner.dsClient, msg)
	case checkType == SubjectSearch:
		result = subjectSearchV1(ctx, runner.dsClient, msg)
	case checkType == ResourceSearch:
		result = resourceSearchV1(ctx, runner.dsClient, msg)
	case checkType == ActionSearch:
		result = actionSearchV1(ctx, runner.dsClient, msg)
	}

	return result
}

const (
	msgVersionUnknown int = iota
	msgVersionV1
	msgVersionV2
	msgVersionV3
)

// reqVersionFields orders the fields that identify a request's shape. Only
// the "Check" check type actually branches on the returned version (its v3
// case); every other check type just needs a non-zero result to pass the
// generic well-formedness gate in exec(), which is why the AuthZEN search
// requests ("resource", no "action" - action_search searches for actions
// rather than taking one) and the batch evaluations request ("evaluations",
// no top-level "action" since each entry is self-contained) are both mapped
// to msgVersionV1 here rather than a version of their own.
var reqVersionFields = []struct {
	name    string
	version int
}{
	{"object_type", msgVersionV3},
	{"object", msgVersionV2},
	{"identity_context", msgVersionV2},
	{"action", msgVersionV1},
	{"resource", msgVersionV1},
	{"evaluations", msgVersionV1},
}

func getReqVersion(val *structpb.Value) int {
	v, ok := val.GetKind().(*structpb.Value_StructValue)
	if !ok {
		return msgVersionUnknown
	}

	fields := v.StructValue.GetFields()

	for _, f := range reqVersionFields {
		if _, ok := fields[f.name]; ok {
			return f.version
		}
	}

	return msgVersionUnknown
}

func checkV3(ctx context.Context, c *dsc.Client, msg *structpb.Value) *CheckResult {
	if c == nil {
		return &CheckResult{
			Outcome:  false,
			Duration: 0,
			Err:      ErrSkippedDirectoryAssertion,
			Str:      skipped,
		}
	}

	var req dsr.CheckRequest
	if err := UnmarshalReq(msg, &req); err != nil {
		return &CheckResult{Err: err}
	}

	start := time.Now()

	resp, err := c.Reader.Check(ctx, &req)

	duration := time.Since(start)

	return &CheckResult{
		Outcome:  resp.GetCheck(),
		Duration: duration,
		Err:      err,
		Str:      checkStringV3(&req),
	}
}

func evaluationV1(ctx context.Context, c *dsc.Client, msg *structpb.Value) *CheckResult {
	if c == nil {
		return &CheckResult{
			Outcome:  false,
			Duration: 0,
			Err:      ErrSkippedDirectoryAssertion,
			Str:      skipped,
		}
	}

	var req dsa.EvaluationRequest
	if err := UnmarshalReq(msg, &req); err != nil {
		return &CheckResult{Err: err}
	}

	start := time.Now()

	resp, err := c.Access.Evaluation(ctx, &req)

	duration := time.Since(start)

	return &CheckResult{
		Outcome:  resp.GetDecision(),
		Duration: duration,
		Err:      err,
		Str:      checkEvaluationStringV1(&req),
	}
}

// authorizerEvaluationV1 asks the same AuthZEN Access Evaluation question as
// evaluationV1, but of the authorizer's policy-engine-backed Access API
// (daemon/authorizer/impl/access.go) rather than the directory's
// graph-backed one - so it exercises the Rego decision itself, action.name
// naming the rule under the loaded policy's root, not a directory relation
// or permission.
func authorizerEvaluationV1(ctx context.Context, c *azc.Client, msg *structpb.Value) *CheckResult {
	if c == nil {
		return &CheckResult{
			Outcome:  false,
			Duration: 0,
			Err:      ErrSkippedAuthorizerAssertion,
			Str:      skipped,
		}
	}

	var req dsa.EvaluationRequest
	if err := UnmarshalReq(msg, &req); err != nil {
		return &CheckResult{Err: err}
	}

	start := time.Now()

	resp, err := c.Access.Evaluation(ctx, &req)

	duration := time.Since(start)

	return &CheckResult{
		Outcome:  resp.GetDecision(),
		Duration: duration,
		Err:      err,
		Str:      checkEvaluationStringV1(&req),
	}
}

// evaluationsV1 exercises the batch "boxcarring" /evaluations endpoint.
// Unlike the single-decision checks above, its outcome is a decision per
// entry in the request's evaluations list, so it passes when that decision
// sequence matches the assertion's expected_decisions, position for
// position - not against the top-level "expected" bool, which stays true.
func evaluationsV1(ctx context.Context, c *dsc.Client, msg *structpb.Struct) *CheckResult {
	if c == nil {
		return &CheckResult{Outcome: false, Duration: 0, Err: ErrSkippedDirectoryAssertion, Str: skipped}
	}

	var req dsa.EvaluationsRequest
	if err := UnmarshalReq(msg.GetFields()[EvaluationsStr], &req); err != nil {
		return &CheckResult{Err: err}
	}

	start := time.Now()

	resp, err := c.Access.Evaluations(ctx, &req)

	duration := time.Since(start)

	actual := decisionsOf(resp.GetEvaluations())
	expected := GetBoolSlice(msg, ExpectedDecisions)

	return &CheckResult{
		Outcome:  boolSliceEqual(actual, expected),
		Duration: duration,
		Err:      err,
		Str:      fmt.Sprintf("evaluations -> %v (want %v)", actual, expected),
	}
}

// subjectSearchV1, resourceSearchV1 and actionSearchV1 exercise the
// AuthZEN search endpoints. Each returns a list rather than a single
// decision, so - like evaluationsV1 - they pass when the result set matches
// the assertion's expected_set as a set (order does not matter), and the
// top-level "expected" bool stays true.
func subjectSearchV1(ctx context.Context, c *dsc.Client, msg *structpb.Struct) *CheckResult {
	if c == nil {
		return &CheckResult{Outcome: false, Duration: 0, Err: ErrSkippedDirectoryAssertion, Str: skipped}
	}

	var req dsa.SubjectSearchRequest
	if err := UnmarshalReq(msg.GetFields()[SubjectSearchStr], &req); err != nil {
		return &CheckResult{Err: err}
	}

	start := time.Now()

	resp, err := c.Access.SubjectSearch(ctx, &req)

	duration := time.Since(start)

	actual := make([]string, 0, len(resp.GetResults()))
	for _, s := range resp.GetResults() {
		actual = append(actual, s.GetType()+":"+s.GetId())
	}

	expected := GetStringSlice(msg, ExpectedSet)

	str := fmt.Sprintf("subject_search %s#%s@%s -> %v (want %v)",
		req.GetResource().GetType(), req.GetResource().GetId(), req.GetAction().GetName(), actual, expected)

	return &CheckResult{
		Outcome:  stringSetEqual(actual, expected),
		Duration: duration,
		Err:      err,
		Str:      str,
	}
}

func resourceSearchV1(ctx context.Context, c *dsc.Client, msg *structpb.Struct) *CheckResult {
	if c == nil {
		return &CheckResult{Outcome: false, Duration: 0, Err: ErrSkippedDirectoryAssertion, Str: skipped}
	}

	var req dsa.ResourceSearchRequest
	if err := UnmarshalReq(msg.GetFields()[ResourceSearchStr], &req); err != nil {
		return &CheckResult{Err: err}
	}

	start := time.Now()

	resp, err := c.Access.ResourceSearch(ctx, &req)

	duration := time.Since(start)

	actual := make([]string, 0, len(resp.GetResults()))
	for _, r := range resp.GetResults() {
		actual = append(actual, r.GetType()+":"+r.GetId())
	}

	expected := GetStringSlice(msg, ExpectedSet)

	str := fmt.Sprintf("resource_search %s#%s@%s -> %v (want %v)",
		req.GetResource().GetType(), req.GetAction().GetName(), req.GetSubject().GetId(), actual, expected)

	return &CheckResult{
		Outcome:  stringSetEqual(actual, expected),
		Duration: duration,
		Err:      err,
		Str:      str,
	}
}

func actionSearchV1(ctx context.Context, c *dsc.Client, msg *structpb.Struct) *CheckResult {
	if c == nil {
		return &CheckResult{Outcome: false, Duration: 0, Err: ErrSkippedDirectoryAssertion, Str: skipped}
	}

	var req dsa.ActionSearchRequest
	if err := UnmarshalReq(msg.GetFields()[ActionSearchStr], &req); err != nil {
		return &CheckResult{Err: err}
	}

	start := time.Now()

	resp, err := c.Access.ActionSearch(ctx, &req)

	duration := time.Since(start)

	actual := make([]string, 0, len(resp.GetResults()))
	for _, a := range resp.GetResults() {
		actual = append(actual, a.GetName())
	}

	expected := GetStringSlice(msg, ExpectedSet)

	str := fmt.Sprintf("action_search %s:%s@%s:%s -> %v (want %v)",
		req.GetResource().GetType(), req.GetResource().GetId(), req.GetSubject().GetType(), req.GetSubject().GetId(), actual, expected)

	return &CheckResult{
		Outcome:  stringSetEqual(actual, expected),
		Duration: duration,
		Err:      err,
		Str:      str,
	}
}

func decisionsOf(evals []*dsa.EvaluationResponse) []bool {
	out := make([]bool, 0, len(evals))
	for _, e := range evals {
		out = append(out, e.GetDecision())
	}

	return out
}

// stringSetEqual reports whether a and b contain the same strings, ignoring
// order and duplicate count position (but not duplicate count itself).
func stringSetEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	counts := make(map[string]int, len(a))
	for _, v := range a {
		counts[v]++
	}

	for _, v := range b {
		counts[v]--
	}

	for _, c := range counts {
		if c != 0 {
			return false
		}
	}

	return true
}

func boolSliceEqual(a, b []bool) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func checkStringV3(req *dsr.CheckRequest) string {
	return fmt.Sprintf("%s:%s#%s@%s:%s",
		req.GetObjectType(), req.GetObjectId(),
		req.GetRelation(),
		req.GetSubjectType(), req.GetSubjectId(),
	)
}

func checkEvaluationStringV1(req *dsa.EvaluationRequest) string {
	return fmt.Sprintf("%s:%s#%s@%s:%s",
		req.GetResource().GetType(), req.GetResource().GetId(),
		req.GetAction().GetName(),
		req.GetSubject().GetType(), req.GetSubject().GetId(),
	)
}
