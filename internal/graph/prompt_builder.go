package graph

import (
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// Registry
// ---------------------------------------------------------------------------

// PromptBuilderRegistry returns the correct PromptBuilder for a category,
// falling back to the general builder when no specific one is registered.
type PromptBuilderRegistry struct {
	builders map[Category]PromptBuilder
	fallback PromptBuilder
}

// NewPromptBuilderRegistry returns a registry pre-loaded with all built-in
// domain builders. Callers can register additional builders via Register.
func NewPromptBuilderRegistry() *PromptBuilderRegistry {
	r := &PromptBuilderRegistry{
		builders: make(map[Category]PromptBuilder),
		fallback: &generalPromptBuilder{},
	}
	r.Register(&techAIPromptBuilder{})
	r.Register(&financePromptBuilder{})
	r.Register(&sciencePromptBuilder{})
	r.Register(&businessPromptBuilder{})
	return r
}

// Register adds or replaces the builder for its category.
func (r *PromptBuilderRegistry) Register(b PromptBuilder) {
	r.builders[b.Category()] = b
}

// For returns the builder for the given category, or the general fallback.
func (r *PromptBuilderRegistry) For(cat Category) PromptBuilder {
	if b, ok := r.builders[cat]; ok {
		return b
	}
	return r.fallback
}

// ---------------------------------------------------------------------------
// Shared prompt skeleton
// ---------------------------------------------------------------------------

// entityPromptTemplate is the shared structure. Domain builders supply the
// type descriptions block and examples; everything else is identical.
func buildEntityPromptFromParts(req ExtractionRequest, typeDescriptions, rules string) string {
	return fmt.Sprintf(`Extract named entities from the following video content.

Entity types:
%s

Rules:
%s
- Always use the full canonical English name. Never abbreviations or acronyms.
- Names must be reusable across many videos, not specific to this video.
- Return at most 15 entities. Prefer precision over recall.
- Omit: presenter names, channel names, generic terms (introduction, overview, example, tutorial).

Title: %s
Description: %s
Content:
%s`,
		typeDescriptions, rules,
		req.Title, truncate(req.Description, 500), req.ContentText)
}

func buildRelationshipPromptFromParts(req ExtractionRequest, entities []Entity, relationshipDescriptions string) string {
	names := make([]string, len(entities))
	for i, e := range entities {
		names[i] = fmt.Sprintf("- %s (%s)", e.Name, e.Type)
	}
	return fmt.Sprintf(`Given the entities below and the video content, extract explicit relationships between them.

Relationship types:
%s

Rules:
- Only extract relationships directly and explicitly supported by the content.
- Both "from" and "to" must be exact names from the entity list below.
- Do not invent relationships that are merely plausible but not stated.
- Return at most 20 relationships.

Entities:
%s

Title: %s
Content:
%s`,
		relationshipDescriptions,
		strings.Join(names, "\n"),
		req.Title, truncate(req.ContentText, 3000))
}

// universalRelationshipDescriptions is identical across all domains.
const universalRelationshipDescriptions = `- extends       : B is a specialisation or evolution of A
- implements    : B is a concrete realisation of A
- optimizes     : A is used to improve or train B
- used_for      : A is applied in the context of B
- part_of       : A is a component or sub-element of B
- compares_to   : A is explicitly contrasted with B
- introduced_by : A was created or proposed by B`

// ---------------------------------------------------------------------------
// Tech / AI
// ---------------------------------------------------------------------------

type techAIPromptBuilder struct{}

func (b *techAIPromptBuilder) Category() Category { return CategoryTechAI }

func (b *techAIPromptBuilder) EntityPrompt(req ExtractionRequest) string {
	types := `- person        : researcher, author, engineer (e.g. "Yann LeCun", "Geoffrey Hinton")
- organization  : company, university, lab (e.g. "DeepMind", "MIT CSAIL", "Hugging Face")
- tool          : framework, library, language, platform (e.g. "PyTorch", "Python", "Kafka", "Kubernetes")
- method        : algorithm, model architecture, technique (e.g. "gradient descent", "BERT", "Random Forest", "attention mechanism")
- concept       : abstract idea or paradigm (e.g. "transfer learning", "overfitting", "zero-shot learning")
- work          : dataset, benchmark, paper (e.g. "ImageNet", "GLUE", "Attention Is All You Need")
- metric        : evaluation or performance measure (e.g. "F1 score", "perplexity", "BLEU score")`

	rules := `- Prefer "convolutional neural network" over "CNN", "natural language processing" over "NLP".`
	return buildEntityPromptFromParts(req, types, rules)
}

func (b *techAIPromptBuilder) RelationshipPrompt(req ExtractionRequest, entities []Entity) string {
	return buildRelationshipPromptFromParts(req, entities, universalRelationshipDescriptions)
}

// ---------------------------------------------------------------------------
// Finance / Quant
// ---------------------------------------------------------------------------

type financePromptBuilder struct{}

func (b *financePromptBuilder) Category() Category { return CategoryFinance }

func (b *financePromptBuilder) EntityPrompt(req ExtractionRequest) string {
	types := `- person        : investor, trader, economist, author (e.g. "Warren Buffett", "Ray Dalio")
- organization  : bank, exchange, fund, regulator (e.g. "Goldman Sachs", "NYSE", "Federal Reserve")
- tool          : software, terminal, data provider, platform (e.g. "Bloomberg Terminal", "QuantLib", "Python", "Refinitiv")
- method        : strategy, model, technique (e.g. "pairs trading", "VWAP", "Black-Scholes", "mean reversion")
- concept       : market theory or principle (e.g. "market microstructure", "efficient market hypothesis", "alpha decay")
- work          : regulation, paper, index, instrument (e.g. "Basel III", "Fama-French paper", "S&P 500", "Black-Scholes formula")
- metric        : financial or risk measure (e.g. "Sharpe ratio", "Value at Risk", "maximum drawdown", "beta")`

	rules := `- Prefer "high frequency trading" over "HFT", "profit and loss" over "P&L".
- Include specific financial instruments only when they are the subject of analysis, not incidental examples.`
	return buildEntityPromptFromParts(req, types, rules)
}

func (b *financePromptBuilder) RelationshipPrompt(req ExtractionRequest, entities []Entity) string {
	return buildRelationshipPromptFromParts(req, entities, universalRelationshipDescriptions)
}

// ---------------------------------------------------------------------------
// Science
// ---------------------------------------------------------------------------

type sciencePromptBuilder struct{}

func (b *sciencePromptBuilder) Category() Category { return CategoryScience }

func (b *sciencePromptBuilder) EntityPrompt(req ExtractionRequest) string {
	types := `- person        : scientist, researcher (e.g. "Charles Darwin", "Marie Curie")
- organization  : lab, institute, agency (e.g. "CERN", "NIH", "NASA")
- tool          : instrument, software, experimental apparatus (e.g. "CRISPR", "electron microscope", "MATLAB", "AlphaFold")
- method        : experimental or analytical technique (e.g. "polymerase chain reaction", "Monte Carlo simulation", "GWAS", "mass spectrometry")
- concept       : theory, principle, phenomenon (e.g. "natural selection", "quantum entanglement", "homeostasis")
- work          : study, dataset, model organism, project (e.g. "Human Genome Project", "UK Biobank", "Copenhagen study")
- metric        : statistical or scientific measure (e.g. "p-value", "confidence interval", "effect size", "signal-to-noise ratio")`

	rules := `- Prefer full Latin or technical names over abbreviations: "deoxyribonucleic acid" not "DNA" if the full form is used.
- For units and measures, include the named unit only when the measure itself is the subject.`
	return buildEntityPromptFromParts(req, types, rules)
}

func (b *sciencePromptBuilder) RelationshipPrompt(req ExtractionRequest, entities []Entity) string {
	return buildRelationshipPromptFromParts(req, entities, universalRelationshipDescriptions)
}

// ---------------------------------------------------------------------------
// Business
// ---------------------------------------------------------------------------

type businessPromptBuilder struct{}

func (b *businessPromptBuilder) Category() Category { return CategoryEconomy }

func (b *businessPromptBuilder) EntityPrompt(req ExtractionRequest) string {
	types := `- person        : executive, entrepreneur, author (e.g. "Steve Jobs", "Peter Drucker")
- organization  : company, consultancy, industry body (e.g. "McKinsey", "Y Combinator", "World Economic Forum")
- tool          : software, framework, methodology tool (e.g. "Salesforce", "Notion", "OKR framework", "Lean Canvas")
- method        : management technique, process, strategy (e.g. "design thinking", "agile", "jobs-to-be-done", "Blue Ocean strategy")
- concept       : business theory or principle (e.g. "product-market fit", "unit economics", "churn", "moat")
- work          : book, report, standard, model (e.g. "Good to Great", "McKinsey 7-S model", "Porter's Five Forces")
- metric        : business or operational measure (e.g. "customer acquisition cost", "net promoter score", "EBITDA", "burn rate")`

	rules := `- Prefer "customer acquisition cost" over "CAC", "net promoter score" over "NPS".`
	return buildEntityPromptFromParts(req, types, rules)
}

func (b *businessPromptBuilder) RelationshipPrompt(req ExtractionRequest, entities []Entity) string {
	return buildRelationshipPromptFromParts(req, entities, universalRelationshipDescriptions)
}

// ---------------------------------------------------------------------------
// General (fallback)
// ---------------------------------------------------------------------------

type generalPromptBuilder struct{}

func (b *generalPromptBuilder) Category() Category { return CategoryGeneral }

func (b *generalPromptBuilder) EntityPrompt(req ExtractionRequest) string {
	types := `- person        : a named individual
- organization  : company, institution, group
- tool          : software, instrument, platform, or technology used to accomplish work
- method        : technique, process, algorithm, or strategy
- concept       : abstract idea, theory, or domain of knowledge
- work          : named publication, dataset, project, regulation, or standard
- metric        : named measure of performance, quality, or quantity`

	rules := `- Always use the full canonical name, never abbreviations.`
	return buildEntityPromptFromParts(req, types, rules)
}

func (b *generalPromptBuilder) RelationshipPrompt(req ExtractionRequest, entities []Entity) string {
	return buildRelationshipPromptFromParts(req, entities, universalRelationshipDescriptions)
}
