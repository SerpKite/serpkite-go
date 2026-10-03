package serpkite

import (
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// contractPaths are where the OpenAPI contract lives: in the monorepo, then
// the copy the mirror bundles into the public serpkite-go repo
// (scripts/sdk-mirror.sh). The test is skipped only when neither exists.
var contractPaths = []string{"../../backend/api/serp-api.yaml", "openapi/serp-api.yaml"}

type schema struct {
	Type       any                `yaml:"type"`
	Required   []string           `yaml:"required"`
	Properties map[string]*schema `yaml:"properties"`
	Items      *schema            `yaml:"items"`
	Ref        string             `yaml:"$ref"`
	Enum       []string           `yaml:"enum"`
}

// TestContract checks that every hand-written struct has exactly the JSON
// keys of its schema in backend/api/serp-api.yaml, and that required keys are
// never omitempty.
func TestContract(t *testing.T) {
	var raw []byte
	var err error
	for _, p := range contractPaths {
		if raw, err = os.ReadFile(p); err == nil {
			break
		}
	}
	if err != nil {
		t.Skipf("contract not found (%v)", err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]*schema `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	s := doc.Components.Schemas
	prop := func(name string, path ...string) *schema {
		cur := s[name]
		for _, p := range path {
			if p == "[]" {
				cur = cur.Items
			} else {
				cur = cur.Properties[p]
			}
			if cur == nil {
				t.Fatalf("schema %s.%s not found", name, strings.Join(path, "."))
			}
		}
		return cur
	}

	cases := map[string]struct {
		schema *schema
		typ    any
	}{
		"SearchRequest":           {prop("SearchRequest"), SearchParams{}},
		"ReviewsRequest":          {prop("ReviewsRequest"), ReviewsParams{}},
		"WebpageRequest":          {prop("WebpageRequest"), WebpageParams{}},
		"RankRequest":             {prop("RankRequest"), RankParams{}},
		"BatchCreateRequest":      {prop("BatchCreateRequest"), BatchCreateParams{}},
		"Meta":                    {prop("Meta"), Meta{}},
		"RouteStep":               {prop("RouteStep"), RouteStep{}},
		"RequestEcho":             {prop("RequestEcho"), RequestEcho{}},
		"Sitelink":                {prop("Sitelink"), Sitelink{}},
		"OrganicResult":           {prop("OrganicResult"), OrganicResult{}},
		"AnswerBox":               {prop("AnswerBox"), AnswerBox{}},
		"KnowledgeGraph":          {prop("KnowledgeGraph"), KnowledgeGraph{}},
		"PeopleAlsoAsk":           {prop("PeopleAlsoAsk"), PeopleAlsoAsk{}},
		"RelatedSearch":           {prop("RelatedSearch"), RelatedSearch{}},
		"SearchResponse":          {prop("SearchResponse"), SearchResponse{}},
		"ImageResult":             {prop("ImageResult"), ImageResult{}},
		"ImagesResponse":          {prop("ImagesResponse"), ImagesResponse{}},
		"VideoResult":             {prop("VideoResult"), VideoResult{}},
		"VideosResponse":          {prop("VideosResponse"), VideosResponse{}},
		"NewsResult":              {prop("NewsResult"), NewsResult{}},
		"NewsResponse":            {prop("NewsResponse"), NewsResponse{}},
		"PlaceResult":             {prop("PlaceResult"), PlaceResult{}},
		"PlacesResponse":          {prop("PlacesResponse"), PlacesResponse{}},
		"ReviewResult":            {prop("ReviewResult"), ReviewResult{}},
		"ReviewResult.user":       {prop("ReviewResult", "user"), ReviewUser{}},
		"ReviewResult.resp":       {prop("ReviewResult", "response"), ReviewReply{}},
		"ReviewsResponse":         {prop("ReviewsResponse"), ReviewsResponse{}},
		"ShoppingResult":          {prop("ShoppingResult"), ShoppingResult{}},
		"ShoppingResponse":        {prop("ShoppingResponse"), ShoppingResponse{}},
		"ScholarResult":           {prop("ScholarResult"), ScholarResult{}},
		"ScholarResponse":         {prop("ScholarResponse"), ScholarResponse{}},
		"PatentResult":            {prop("PatentResult"), PatentResult{}},
		"PatentsResponse":         {prop("PatentsResponse"), PatentsResponse{}},
		"Suggestion":              {prop("Suggestion"), Suggestion{}},
		"AutocompleteResponse":    {prop("AutocompleteResponse"), AutocompleteResponse{}},
		"PageMetadata":            {prop("PageMetadata"), PageMetadata{}},
		"WebpageResponse":         {prop("WebpageResponse"), WebpageResponse{}},
		"PageLink":                {prop("PageLink"), PageLink{}},
		"Highlight":               {prop("Highlight"), Highlight{}},
		"MapRequest":              {prop("MapRequest"), MapParams{}},
		"MapURL":                  {prop("MapURL"), MapURL{}},
		"MapResponse":             {prop("MapResponse"), MapResponse{}},
		"MapResponse.request":     {prop("MapResponse", "request"), MapRequestEcho{}},
		"MapResponse.meta":        {prop("MapResponse", "meta"), MapMeta{}},
		"ExtractRequest":          {prop("ExtractRequest"), ExtractParams{}},
		"ExtractResult":           {prop("ExtractResult"), ExtractResult{}},
		"ExtractFailure":          {prop("ExtractFailure"), ExtractFailure{}},
		"ExtractFailure.error":    {prop("ExtractFailure", "error"), ExtractFailureCode{}},
		"ExtractResponse":         {prop("ExtractResponse"), ExtractResponse{}},
		"ExtractResponse.request": {prop("ExtractResponse", "request"), ExtractRequestEcho{}},
		"ExtractResponse.meta":    {prop("ExtractResponse", "meta"), ExtractMeta{}},
		"RankResponse":            {prop("RankResponse"), RankResponse{}},
		"RankResponse.matches":    {prop("RankResponse", "matches", "[]"), RankMatch{}},
		"Account":                 {prop("Account"), Account{}},
		"Account.key":             {prop("Account", "key"), AccountKey{}},
		"Account.month":           {prop("Account", "month"), AccountMonth{}},
		"Batch":                   {prop("Batch"), Batch{}},
		"Batch.error":             {prop("Batch", "error"), BatchError{}},
		"CrawlRequest":            {prop("CrawlRequest"), CrawlParams{}},
		"TaskCreated":             {prop("TaskCreated"), TaskCreated{}},
		"TaskCancelResponse":      {prop("TaskCancelResponse"), TaskCancelResponse{}},
		"TaskError":               {prop("TaskError"), TaskError{}},
		"CrawlPage":               {prop("CrawlPage"), CrawlPage{}},
		"CrawlResult":             {prop("CrawlResult"), CrawlResult{}},
		"CrawlResult.stats":       {prop("CrawlResult", "stats"), CrawlStats{}},
		"CrawlTask":               {prop("CrawlTask"), CrawlTask{}},
		"CrawlTask.progress":      {prop("CrawlTask", "progress"), CrawlProgress{}},
		"TaskCompletedEvent":      {prop("TaskCompletedEvent"), TaskCompletedEvent{}},
		"TaskCompletedEvent.err":  {prop("TaskCompletedEvent", "error"), TaskError{}},
		"MonitorSearch":           {prop("MonitorSearch"), MonitorSearch{}},
		"MonitorCreateRequest":    {prop("MonitorCreateRequest"), MonitorCreateParams{}},
		"MonitorUpdateRequest":    {prop("MonitorUpdateRequest"), MonitorUpdateParams{}},
		"Monitor":                 {prop("Monitor"), Monitor{}},
		"MonitorList":             {prop("MonitorList"), MonitorList{}},
		"MonitorRun":              {prop("MonitorRun"), MonitorRun{}},
		"MonitorPageChange":       {prop("MonitorPageChange"), MonitorPageChange{}},
		"MonitorRunList":          {prop("MonitorRunList"), MonitorRunList{}},
		"MonitorResultsEvent":     {prop("MonitorResultsEvent"), MonitorResultsEvent{}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			want := make([]string, 0, len(c.schema.Properties))
			for k := range c.schema.Properties {
				want = append(want, k)
			}
			got, omit := jsonKeys(reflect.TypeOf(c.typ))
			slices.Sort(want)
			slices.Sort(got)
			for _, k := range want {
				if !slices.Contains(got, k) {
					t.Errorf("%T is missing %q", c.typ, k)
				}
			}
			for _, k := range got {
				if !slices.Contains(want, k) {
					t.Errorf("%T has %q, which the contract does not define", c.typ, k)
				}
			}
			for _, k := range c.schema.Required {
				if omit[k] {
					t.Errorf("%T: required %q must not be omitempty", c.typ, k)
				}
			}
		})
	}

	// The Provider constants match the contract's Provider enum.
	providers := []string{ProviderGoogle, ProviderBrave, ProviderBing, ProviderYahoo, ProviderDuckDuckGo, ProviderMojeek, ProviderWikipedia}
	if p := s["Provider"]; p == nil || !slices.Equal(p.Enum, providers) {
		t.Errorf("Provider constants %v differ from the contract enum %v", providers, p)
	}

	// Every response schema is covered.
	for name := range s {
		if strings.HasSuffix(name, "Response") && name != "CSEResponse" && name != "BatchCreateResponse" {
			if _, ok := cases[name]; !ok {
				t.Errorf("schema %s has no Go type in contract_test.go", name)
			}
		}
	}
}

func jsonKeys(t reflect.Type) ([]string, map[string]bool) {
	var keys []string
	omit := map[string]bool{}
	for f := range t.Fields() {
		tag, ok := f.Tag.Lookup("json")
		if !ok || tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")
		keys = append(keys, name)
		omit[name] = strings.Contains(opts, "omitempty")
	}
	return keys, omit
}
