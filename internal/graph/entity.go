package graph

// EntityType classifies the kind of entity using universal types that are
// meaningful across all domains (tech/AI, finance, science, business, etc.).
type EntityType string

const (
	// EntityTypePerson is a named individual: researcher, author, practitioner.
	EntityTypePerson EntityType = "person"

	// EntityTypeOrganization is a company, university, or research lab.
	EntityTypeOrganization EntityType = "organization"

	// EntityTypeTool is a concrete artifact used to accomplish work:
	// framework, library, programming language, software, instrument, platform.
	// Tech: "PyTorch", "Python". Finance: "Bloomberg Terminal". Science: "CRISPR".
	EntityTypeTool EntityType = "tool"

	// EntityTypeMethod is a systematic procedure or model:
	// algorithm, architecture, trading strategy, experimental technique.
	// Tech: "gradient descent", "BERT". Finance: "pairs trading". Science: "PCR".
	EntityTypeMethod EntityType = "method"

	// EntityTypeConcept is an abstract idea, theory, or domain of knowledge.
	// Tech: "transfer learning". Finance: "market microstructure". Science: "natural selection".
	EntityTypeConcept EntityType = "concept"

	// EntityTypeWork is a named artifact of intellectual output:
	// dataset, benchmark, paper, book, regulation, standard, financial instrument.
	// Tech: "ImageNet". Finance: "Basel III". Science: "Human Genome Project".
	EntityTypeWork EntityType = "work"

	// EntityTypeMetric is a named measure of performance or quality.
	// Tech: "F1 score". Finance: "Sharpe ratio". Science: "p-value".
	EntityTypeMetric EntityType = "metric"
)

// Category is the high-level domain a piece of content belongs to.
// Drives which PromptBuilder is selected and scopes entity deduplication.
type Category string

const (
	CategoryTechAI  Category = "tech_ai"
	CategoryFinance Category = "finance"
	CategoryScience Category = "science"
	CategoryEconomy Category = "economy"
	CategoryGeneral Category = "general"
)

// AllCategories is the ordered list of valid categories used for LLM classification.
var AllCategories = []Category{
	CategoryTechAI,
	CategoryFinance,
	CategoryScience,
	CategoryEconomy,
	CategoryGeneral,
}

// RelationshipType is the directed semantic edge label between two entities.
// Universal across all domains.
type RelationshipType string

const (
	RelExtends      RelationshipType = "extends"       // BERT extends Transformer
	RelImplements   RelationshipType = "implements"    // PyTorch implements Autograd
	RelOptimizes    RelationshipType = "optimizes"     // Adam optimizes NeuralNetwork
	RelUsedFor      RelationshipType = "used_for"      // Python used_for DataScience
	RelPartOf       RelationshipType = "part_of"       // Attention part_of Transformer
	RelComparesTo   RelationshipType = "compares_to"   // SGD compares_to Adam
	RelIntroducedBy RelationshipType = "introduced_by" // Transformer introduced_by Google
)

// Entity is a named node extracted from video content.
type Entity struct {
	// Key is the canonical, normalised identifier used as the graph vertex key.
	// Derived from Name: lowercased, whitespace collapsed, non-alphanumeric stripped.
	Key  string
	Name string
	Type EntityType

	// Categories accumulates all domains this entity has been observed in across
	// videos. An entity gains categories over time; they are never reset per extraction.
	// e.g. "monte_carlo" starts as ["finance"], gains ["science"] later.
	Categories []Category

	// Embedding is the vector representation of the entity name used for semantic
	// deduplication. Tagged json:"-" so generic upserts never write it — use
	// StoreEntityEmbedding for that.
	Embedding []float32 `json:"-"`
}

// Relationship is a directed, typed edge between two entities within one
// extraction result. FromKey and ToKey reference Entity.Key values.
type Relationship struct {
	FromKey string
	ToKey   string
	Type    RelationshipType
}
