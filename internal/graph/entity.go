package graph

// EntityType classifies the kind of entity.
type EntityType string

const (
	EntityTypeTopic   EntityType = "topic"
	EntityTypeConcept EntityType = "concept"
)

// Entity is a named node extracted from video content.
type Entity struct {
	// Key is the canonical, normalised identifier used as the graph vertex key.
	// Derived from Name: lowercased, whitespace collapsed, non-alphanumeric stripped.
	Key  string
	Name string
	Type EntityType
}
