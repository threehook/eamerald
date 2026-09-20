//nolint:testpackage
package directory

import (
	"strings"
	"testing"

	dsi "github.com/aserto-dev/go-directory/aserto/directory/importer/v3"
	"github.com/stretchr/testify/require"
)

type fakeImportStream struct {
	dsi.Importer_ImportClient

	sent []*dsi.ImportRequest
}

func (f *fakeImportStream) Send(req *dsi.ImportRequest) error {
	f.sent = append(f.sent, req)
	return nil
}

func TestImportFromReader_ClassifiesRelationsCorrectly(t *testing.T) {
	input := `{"type":"course","id":"laadpalen-management","properties":{"validity":"P1Y"}}
{"object_type":"department","object_id":"burgerzaken","relation":"member","subject_type":"user","subject_id":"jerry@example.com"}
`

	stream := &fakeImportStream{}
	c := &Client{}

	require.NoError(t, c.importFromReader(stream, strings.NewReader(input)))
	require.Len(t, stream.sent, 2)

	_, isObj := stream.sent[0].GetMsg().(*dsi.ImportRequest_Object)
	require.True(t, isObj, "object line should be classified as an object")

	rel, isRel := stream.sent[1].GetMsg().(*dsi.ImportRequest_Relation)
	require.True(t, isRel, "relation line should be classified as a relation, not silently sent as an empty object")
	require.Equal(t, "burgerzaken", rel.Relation.GetObjectId())
	require.Equal(t, "member", rel.Relation.GetRelation())
	require.Equal(t, "jerry@example.com", rel.Relation.GetSubjectId())
}
