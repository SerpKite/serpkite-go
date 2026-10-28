package serpkite

// Types of the AI assistant endpoints (/v1/chatgpt, /v1/gemini, /v1/copilot,
// /v1/ai_mode, /v1/brave_ai) and of Google's AI Overview. Like types.go they
// mirror backend/api/serp-api.yaml and are checked by contract_test.go.

// AI assistant endpoints, for [Client.Markdown], batches and monitors.
const (
	EndpointChatGPT Endpoint = "chatgpt"
	EndpointGemini  Endpoint = "gemini"
	EndpointCopilot Endpoint = "copilot"
	EndpointAIMode  Endpoint = "ai_mode"
	EndpointBraveAI Endpoint = "brave_ai"
)

// AssistantParams are the parameters of [Client.Gemini], [Client.Copilot],
// [Client.AIMode] and [Client.BraveAI].
type AssistantParams struct {
	// Prompt is the message to the assistant (required, at most 4000 characters).
	Prompt string `json:"prompt"`
	// Country asks from this country (ISO 3166-1 alpha-2): answers, prices
	// and shops follow it.
	Country string `json:"country,omitempty"`
	// Language is the interface language (e.g. "en", "de"); the answer
	// follows the prompt's language.
	Language string `json:"language,omitempty"`
	// Track lists brands ("Acme") and domains ("acme.com", subdomains match),
	// at most 20, to check the answer for; each comes back in
	// [AssistantResponse].Mentions.
	Track []string `json:"track,omitempty"`
	// IncludeHTML also returns the answer page's HTML.
	IncludeHTML bool `json:"include_html,omitempty"`
	// Format is "json" (default), "markdown" or "compact".
	Format string `json:"format,omitempty"`
	Fields string `json:"fields,omitempty"`
	// MaxAge accepts a cached answer up to this many seconds old (half
	// price); answers are cached for at most an hour.
	MaxAge int `json:"max_age,omitempty"`
}

// ChatGPTParams are the parameters of [Client.ChatGPT]: like
// [AssistantParams] plus WebSearch, without Language.
type ChatGPTParams struct {
	// Prompt is the message to ChatGPT (required, at most 4000 characters).
	Prompt string `json:"prompt"`
	// WebSearch makes ChatGPT search the web (default true); Bool(false)
	// lets it decide.
	WebSearch *bool `json:"web_search,omitempty"`
	// Country asks from an exit in this country (residential proxies).
	// Without it any of our exits asks (mostly US and EU).
	Country     string   `json:"country,omitempty"`
	Track       []string `json:"track,omitempty"`
	IncludeHTML bool     `json:"include_html,omitempty"`
	Format      string   `json:"format,omitempty"`
	Fields      string   `json:"fields,omitempty"`
	MaxAge      int      `json:"max_age,omitempty"`
}

// AssistantEcho is the normalised request of an AI assistant response (the
// path names the assistant, so there is no engine).
type AssistantEcho struct {
	Endpoint string `json:"endpoint"`
	Prompt   string `json:"prompt"`
	Country  string `json:"country,omitempty"`
	// Language is not echoed by chatgpt.
	Language string `json:"language,omitempty"`
	// WebSearch is echoed by chatgpt only.
	WebSearch   *bool `json:"web_search,omitempty"`
	IncludeHTML bool  `json:"include_html,omitempty"`
}

// AISource is a source of an AI answer or AI Overview.
type AISource struct {
	// Position is the number the answer cites it with ([n] in Markdown).
	Position int    `json:"position"`
	Title    string `json:"title,omitempty"`
	Link     string `json:"link"`
	Domain   string `json:"domain"`
	Snippet  string `json:"snippet,omitempty"`
	Date     string `json:"date,omitempty"`
	// Cited is true when the answer cites it inline (false: only listed beside it).
	Cited bool `json:"cited"`
}

// AIProduct is a product an AI assistant showed.
type AIProduct struct {
	Position int    `json:"position"`
	Title    string `json:"title"`
	Brand    string `json:"brand,omitempty"`
	// Price is as shown, e.g. "€69.90".
	Price          string  `json:"price,omitempty"`
	ExtractedPrice float64 `json:"extracted_price,omitempty"`
	// Currency is ISO 4217 when the symbol is unambiguous.
	Currency    string  `json:"currency,omitempty"`
	Rating      float64 `json:"rating,omitempty"`
	Reviews     int     `json:"reviews,omitempty"`
	Merchant    string  `json:"merchant,omitempty"`
	Link        string  `json:"link,omitempty"`
	Image       string  `json:"image,omitempty"`
	Description string  `json:"description,omitempty"`
	Sponsored   bool    `json:"sponsored,omitempty"`
}

// Mention is one Track term checked against an AI answer.
type Mention struct {
	Term string `json:"term"`
	// Mentioned: the answer text names it (a domain: names the domain).
	Mentioned bool `json:"mentioned"`
	// Cited: a cited source matches it.
	Cited bool `json:"cited"`
	// Sources are the positions of the matching sources.
	Sources []int `json:"sources"`
	// Products are the positions of the matching products.
	Products []int `json:"products,omitempty"`
	// TextPosition is where the text first names it, 0 (start) to 1 (end);
	// -1 when it doesn't.
	TextPosition float64 `json:"text_position"`
}

// AssistantResponse is returned by [Client.ChatGPT], [Client.Gemini],
// [Client.Copilot], [Client.AIMode] and [Client.BraveAI].
type AssistantResponse struct {
	Request AssistantEcho `json:"request"`
	// Text is the answer as plain text.
	Text string `json:"text"`
	// Markdown is the answer with citations as [n](link), matching
	// Sources[].Position. With Format "markdown" only Markdown is filled in.
	Markdown string `json:"markdown"`
	// HTML is set with IncludeHTML.
	HTML string `json:"html,omitempty"`
	// Model is the model the assistant reports, when it does.
	Model   string     `json:"model,omitempty"`
	Sources []AISource `json:"sources"`
	// SearchQueries are the searches the assistant ran, where it shows them.
	SearchQueries []string    `json:"search_queries"`
	Products      []AIProduct `json:"products"`
	// Places are the local businesses the answer showed.
	Places           []PlaceResult `json:"places,omitempty"`
	RelatedQuestions []string      `json:"related_questions,omitempty"`
	// Mentions is set with Track.
	Mentions []Mention `json:"mentions,omitempty"`
	Meta     Meta      `json:"meta"`
}

// AIOverview is Google's AI Overview on a search page ([SearchResponse].AIOverview).
type AIOverview struct {
	Text string `json:"text"`
	// Markdown has citations as [n](link), matching Sources[].Position.
	Markdown string     `json:"markdown"`
	Sources  []AISource `json:"sources"`
}

// MonitorAnswer is what an AI assistant monitor reports every run (one item
// in [MonitorRun].Results and [MonitorResultsEvent].NewResults).
type MonitorAnswer struct {
	Text     string     `json:"text"`
	Markdown string     `json:"markdown"`
	Model    string     `json:"model,omitempty"`
	Sources  []AISource `json:"sources"`
	// NewSources are the links of the sources not seen in earlier runs.
	NewSources    []string    `json:"new_sources"`
	SearchQueries []string    `json:"search_queries"`
	Products      []AIProduct `json:"products,omitempty"`
	// Mentions is set when the monitor has Track.
	Mentions []Mention `json:"mentions,omitempty"`
}
