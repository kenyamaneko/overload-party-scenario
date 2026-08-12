package domain_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/kenyamaneko/overload-party-scenario/internal/domain"
)

func TestLockReasonTypeAgainstOpenAPISpec(t *testing.T) {
	t.Run("[シナリオ]LockReasonTypeとdata/openapi.yamlの整合", func(t *testing.T) {
		t.Run("LockReasonTypeの値集合がdata/openapi.yamlのLockReasonType enumの値集合と一致する", func(t *testing.T) {
			spec := loadOpenAPISpec(t)
			want := []string{
				string(domain.LockReasonLevel),
				string(domain.LockReasonFaction),
				string(domain.LockReasonEpisode),
			}
			got := specEnumValues(t, spec, "LockReasonType")
			require.ElementsMatch(t, want, got)
		})
	})
}

// loadOpenAPISpec は data/openapi.yaml をパースして返す。
func loadOpenAPISpec(t *testing.T) map[string]interface{} {
	t.Helper()
	specPath := filepath.Join(repoRoot(t), "data", "openapi.yaml")
	raw, err := os.ReadFile(specPath)
	require.NoError(t, err)
	var doc map[string]interface{}
	require.NoError(t, yaml.Unmarshal(raw, &doc))
	return doc
}

// specEnumValues は components/schemas/<name>/enum 配下の値一覧を取り出す。
func specEnumValues(t *testing.T, spec map[string]interface{}, schemaName string) []string {
	t.Helper()
	components, ok := spec["components"].(map[string]interface{})
	require.True(t, ok, "components が見つからない")
	schemas, ok := components["schemas"].(map[string]interface{})
	require.True(t, ok, "components/schemas が見つからない")
	schema, ok := schemas[schemaName].(map[string]interface{})
	require.True(t, ok, "components/schemas/%s が見つからない", schemaName)
	rawEnum, ok := schema["enum"].([]interface{})
	require.True(t, ok, "components/schemas/%s/enum が無い、または配列でない", schemaName)
	out := make([]string, 0, len(rawEnum))
	for _, v := range rawEnum {
		s, ok := v.(string)
		require.True(t, ok, "%s の enum 値が文字列でない", schemaName)
		out = append(out, s)
	}
	return out
}

// repoRoot は本ファイルから見たリポジトリルートを返す (internal/domain/ から 2 階層上)。
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Join(wd, "..", "..")
}
