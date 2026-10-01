package types_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/domain/types"
)

func TestFieldType_IsValid(t *testing.T) {
	tests := []struct {
		name      string
		fieldType types.FieldType
		want      bool
	}{
		{
			name:      "valid text",
			fieldType: types.FieldTypeText,
			want:      true,
		},
		{
			name:      "valid number",
			fieldType: types.FieldTypeNumber,
			want:      true,
		},
		{
			name:      "valid select",
			fieldType: types.FieldTypeSelect,
			want:      true,
		},
		{
			name:      "valid multi-select",
			fieldType: types.FieldTypeMultiSelect,
			want:      true,
		},
		{
			name:      "valid user",
			fieldType: types.FieldTypeUser,
			want:      true,
		},
		{
			name:      "valid multi-user",
			fieldType: types.FieldTypeMultiUser,
			want:      true,
		},
		{
			name:      "valid date",
			fieldType: types.FieldTypeDate,
			want:      true,
		},
		{
			name:      "valid url",
			fieldType: types.FieldTypeURL,
			want:      true,
		},
		{
			name:      "valid case_ref",
			fieldType: types.FieldTypeCaseRef,
			want:      true,
		},
		{
			name:      "valid multi_case_ref",
			fieldType: types.FieldTypeMultiCaseRef,
			want:      true,
		},
		{
			name:      "valid markdown",
			fieldType: types.FieldTypeMarkdown,
			want:      true,
		},
		{
			name:      "invalid type",
			fieldType: types.FieldType("invalid"),
			want:      false,
		},
		{
			name:      "empty type",
			fieldType: types.FieldType(""),
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.want {
				gt.B(t, tt.fieldType.IsValid()).True()
			} else {
				gt.B(t, tt.fieldType.IsValid()).False()
			}
		})
	}
}

func TestAllFieldTypes(t *testing.T) {
	fieldTypes := types.AllFieldTypes()
	expectedCount := 11

	gt.A(t, fieldTypes).Length(expectedCount)

	// Verify all returned types are valid
	for _, fieldType := range fieldTypes {
		gt.B(t, fieldType.IsValid()).
			Describef("Field type %s should be valid", fieldType).
			True()
	}

	// Verify all expected types are present
	expectedTypes := []types.FieldType{
		types.FieldTypeText,
		types.FieldTypeNumber,
		types.FieldTypeSelect,
		types.FieldTypeMultiSelect,
		types.FieldTypeUser,
		types.FieldTypeMultiUser,
		types.FieldTypeDate,
		types.FieldTypeURL,
		types.FieldTypeCaseRef,
		types.FieldTypeMultiCaseRef,
		types.FieldTypeMarkdown,
	}

	typeMap := make(map[types.FieldType]bool)
	for _, fieldType := range fieldTypes {
		typeMap[fieldType] = true
	}

	for _, expected := range expectedTypes {
		gt.B(t, typeMap[expected]).
			Describef("Expected field type %s should be present", expected).
			True()
	}
}

func TestFieldType_String(t *testing.T) {
	tests := []struct {
		name      string
		fieldType types.FieldType
		want      string
	}{
		{
			name:      "text",
			fieldType: types.FieldTypeText,
			want:      "text",
		},
		{
			name:      "number",
			fieldType: types.FieldTypeNumber,
			want:      "number",
		},
		{
			name:      "select",
			fieldType: types.FieldTypeSelect,
			want:      "select",
		},
		{
			name:      "multi-select",
			fieldType: types.FieldTypeMultiSelect,
			want:      "multi-select",
		},
		{
			name:      "user",
			fieldType: types.FieldTypeUser,
			want:      "user",
		},
		{
			name:      "multi-user",
			fieldType: types.FieldTypeMultiUser,
			want:      "multi-user",
		},
		{
			name:      "date",
			fieldType: types.FieldTypeDate,
			want:      "date",
		},
		{
			name:      "url",
			fieldType: types.FieldTypeURL,
			want:      "url",
		},
		{
			name:      "case_ref",
			fieldType: types.FieldTypeCaseRef,
			want:      "case_ref",
		},
		{
			name:      "multi_case_ref",
			fieldType: types.FieldTypeMultiCaseRef,
			want:      "multi_case_ref",
		},
		{
			name:      "markdown",
			fieldType: types.FieldTypeMarkdown,
			want:      "markdown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gt.S(t, tt.fieldType.String()).Equal(tt.want)
		})
	}
}

func TestFieldType_IsCaseRef(t *testing.T) {
	tests := []struct {
		name      string
		fieldType types.FieldType
		want      bool
	}{
		{
			name:      "case_ref is true",
			fieldType: types.FieldTypeCaseRef,
			want:      true,
		},
		{
			name:      "multi_case_ref is true",
			fieldType: types.FieldTypeMultiCaseRef,
			want:      true,
		},
		{
			name:      "text is false",
			fieldType: types.FieldTypeText,
			want:      false,
		},
		{
			name:      "user is false",
			fieldType: types.FieldTypeUser,
			want:      false,
		},
		{
			name:      "markdown is false",
			fieldType: types.FieldTypeMarkdown,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.want {
				gt.B(t, tt.fieldType.IsCaseRef()).True()
			} else {
				gt.B(t, tt.fieldType.IsCaseRef()).False()
			}
		})
	}
}

func TestTextPattern_Match(t *testing.T) {
	tests := []struct {
		name    string
		pattern types.TextPattern
		value   string
		want    bool
	}{
		{name: "whole value matches", pattern: "[A-Z]{3}-[0-9]+", value: "ABC-12", want: true},
		{name: "partial match is rejected", pattern: "[A-Z]{3}-[0-9]+", value: "xxABC-12yy", want: false},
		{name: "incomplete value is rejected", pattern: "[A-Z]{3}-[0-9]+", value: "ABC-", want: false},
		{name: "first alternative", pattern: "foo|bar", value: "foo", want: true},
		{name: "second alternative", pattern: "foo|bar", value: "bar", want: true},
		{name: "alternation is grouped before anchoring", pattern: "foo|bar", value: "foobar", want: false},
		{name: "alternation does not match a suffix", pattern: "foo|bar", value: "xfoo", want: false},
		{name: "explicit anchors keep the same meaning", pattern: "^C[0-9]+$", value: "C123", want: true},
		{name: "explicit anchors still reject a partial match", pattern: "^C[0-9]+$", value: "xC123", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.pattern.Match(tt.value)
			gt.NoError(t, err)
			gt.Equal(t, got, tt.want)
		})
	}
}

func TestTextPattern_InvalidSyntax(t *testing.T) {
	p := types.TextPattern("[a-")

	re, err := p.Compile()
	gt.Error(t, err)
	gt.Nil(t, re)

	ok, err := p.Match("a")
	gt.Error(t, err)
	gt.B(t, ok).False()
}

// A pattern that is unbalanced on its own must not be able to close the
// anchoring group and widen the match to any value.
func TestTextPattern_UnbalancedPatternCannotEscapeTheAnchor(t *testing.T) {
	for _, p := range []types.TextPattern{"a)|(.*", ")(", "a)"} {
		re, err := p.Compile()
		gt.Error(t, err)
		gt.Nil(t, re)

		ok, err := p.Match("anything")
		gt.Error(t, err)
		gt.B(t, ok).False()
	}
}

func TestTextPattern_PromptHintAndLabel(t *testing.T) {
	t.Run("empty pattern renders nothing", func(t *testing.T) {
		p := types.TextPattern("")
		gt.Equal(t, p.PromptHint(), "")
		gt.Equal(t, p.Label(), "")
	})

	t.Run("pattern renders backquoted with the matching rule", func(t *testing.T) {
		p := types.TextPattern("^C[0-9]+$")
		gt.Equal(t, p.String(), "^C[0-9]+$")
		gt.Equal(t, p.PromptHint(),
			"the whole value must match this regular expression (Go RE2 syntax); an empty value is always accepted")
		gt.Equal(t, p.Label(),
			"`^C[0-9]+$` — the whole value must match this regular expression (Go RE2 syntax); an empty value is always accepted")
	})
}
