package config

import "github.com/secmon-lab/hecatoncheires/pkg/domain/types"

// FieldOption represents an option for select/multi-select fields
type FieldOption struct {
	ID          string
	Name        string
	Description string
	Metadata    map[string]any // Optional: arbitrary metadata (e.g., {"score": 4})
}

// FieldDefinition defines a custom field's schema
type FieldDefinition struct {
	ID          string
	Name        string
	Type        types.FieldType
	Required    bool
	Description string
	Options     []FieldOption // Only used for select and multi-select types
	// ReferenceWorkspace is the workspace ID whose Cases this field may
	// reference. Required (and only meaningful) for case_ref /
	// multi_case_ref types; empty for all other types. May point at the
	// field's own workspace (self-reference is allowed).
	ReferenceWorkspace string
	// Semantic is how a text field's value is interpreted (e.g. as a Slack
	// channel ID). Only meaningful for the text type; empty means free text.
	Semantic types.Semantic
	// Validation holds the constraints on the shape of the field's value.
	// The zero value imposes no constraint.
	Validation FieldValidation
}

// FieldValidation groups the constraints on the shape of a field's value,
// mirroring the [fields.validation] table in the workspace config. Pattern
// is its only member for now; length bounds belong here when added.
type FieldValidation struct {
	// Pattern is a regular expression the whole value must match. Only
	// meaningful for the text type.
	Pattern types.TextPattern
}

// EntityLabels holds display labels for entities
type EntityLabels struct {
	Case string // Default: "Case"
}

// FieldSchema holds the complete field configuration
type FieldSchema struct {
	Fields []FieldDefinition
	Labels EntityLabels
}
