package config_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gollem-dev/gollem"
	"github.com/m-mizutani/gt"

	agentkernel "github.com/m-mizutani/hecatoncheires/pkg/agent/kernel"
	"github.com/m-mizutani/hecatoncheires/pkg/cli/config"
	"github.com/m-mizutani/hecatoncheires/pkg/utils/pricing"
)

// modelDef builds a priced definition for one provider, which is what
// NewClientFor takes.
func modelDef(provider, model string) agentkernel.ModelDef {
	return agentkernel.ModelDef{
		Ref:      model,
		Provider: provider,
		Model:    model,
		Rate:     pricing.Rate{Input: 1000, Output: 5000},
	}
}

func TestLLM_IsEnabled(t *testing.T) {
	gt.Bool(t, config.NewLLMForTest("", "", "", "", "").IsEnabled()).False()
	gt.Bool(t, config.NewLLMForTest("cheap", "", "", "", "").IsEnabled()).True()
	gt.String(t, config.NewLLMForTest("cheap", "", "", "", "").ModelRef()).Equal("cheap")
}

func TestLLM_NewClientFor_RejectsAnUnpricedDefinition(t *testing.T) {
	cfg := config.NewLLMForTest("cheap", "openai-key", "", "", "")

	_, err := cfg.NewClientFor(context.Background(), agentkernel.ModelDef{
		Ref: "cheap", Provider: agentkernel.ProviderOpenAI, Model: "gpt-4o",
	})

	gt.Error(t, err)
	gt.String(t, err.Error()).Contains("priced at nothing")
}

func TestLLM_OpenAI_RequiresAPIKey(t *testing.T) {
	cfg := config.NewLLMForTest("gpt-4o", "", "", "", "")

	_, err := cfg.NewClientFor(context.Background(), modelDef(agentkernel.ProviderOpenAI, "gpt-4o"))

	gt.Error(t, err)
	gt.String(t, err.Error()).Contains("openai-api-key")
}

func TestLLM_Claude_RequiresCredentials(t *testing.T) {
	cfg := config.NewLLMForTest("claude-opus-5", "", "", "", "global")

	_, err := cfg.NewClientFor(context.Background(),
		modelDef(agentkernel.ProviderClaude, "claude-opus-5"))

	gt.Error(t, err)
	gt.String(t, err.Error()).Contains("claude-api-key")
}

func TestLLM_Claude_RejectsBothCredentials(t *testing.T) {
	cfg := config.NewLLMForTest("claude-opus-5", "", "anthropic-key", "gcp-project", "global")

	_, err := cfg.NewClientFor(context.Background(),
		modelDef(agentkernel.ProviderClaude, "claude-opus-5"))

	gt.Error(t, err)
	gt.String(t, err.Error()).Contains("mutually exclusive")
}

func TestLLM_Claude_VertexRequiresLocation(t *testing.T) {
	cfg := config.NewLLMForTest("claude-opus-5", "", "", "gcp-project", "")

	_, err := cfg.NewClientFor(context.Background(),
		modelDef(agentkernel.ProviderClaude, "claude-opus-5"))

	gt.Error(t, err)
	gt.String(t, err.Error()).Contains("location")
}

// claudeRequestBody builds a Claude API client for def against a fake Messages
// endpoint, makes one Generate, and returns the request body the client sent.
func claudeRequestBody(t *testing.T, def agentkernel.ModelDef) map[string]any {
	t.Helper()
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		gt.NoError(t, err).Required()
		body = raw
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"` + def.Model +
			`","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn",` +
			`"usage":{"input_tokens":1,"output_tokens":1}}`))
		gt.NoError(t, err).Required()
	}))
	t.Cleanup(srv.Close)
	// The SDK reads the endpoint from here when the client names none, which is
	// what NewClientFor's client does.
	t.Setenv("ANTHROPIC_BASE_URL", srv.URL)

	ctx := context.Background()
	cfg := config.NewLLMForTest(def.Ref, "", "anthropic-key", "", "")
	client, err := cfg.NewClientFor(ctx, def)
	gt.NoError(t, err).Required()
	session, err := client.NewSession(ctx)
	gt.NoError(t, err).Required()
	_, err = session.Generate(ctx, []gollem.Input{gollem.Text("hello")})
	gt.NoError(t, err).Required()

	gt.Value(t, body).NotNil().Required()
	var decoded map[string]any
	gt.NoError(t, json.Unmarshal(body, &decoded)).Required()
	return decoded
}

func TestLLM_Claude_SendsTheReasoningSettings(t *testing.T) {
	t.Run("all set", func(t *testing.T) {
		def := modelDef(agentkernel.ProviderClaude, "claude-sonnet-5-5")
		def.Thinking = agentkernel.ThinkingAdaptive
		def.ThinkingDisplay = "summarized"
		def.Effort = "xhigh"

		body := claudeRequestBody(t, def)
		gt.Value(t, body["thinking"]).Equal(map[string]any{"type": "adaptive", "display": "summarized"})
		outputConfig, ok := body["output_config"].(map[string]any)
		gt.Bool(t, ok).True().Required()
		gt.Value(t, outputConfig["effort"]).Equal("xhigh")
	})

	t.Run("adaptive without a display leaves the display to the API", func(t *testing.T) {
		def := modelDef(agentkernel.ProviderClaude, "claude-sonnet-5-5")
		def.Thinking = agentkernel.ThinkingAdaptive

		body := claudeRequestBody(t, def)
		gt.Value(t, body["thinking"]).Equal(map[string]any{"type": "adaptive"})
		_, hasOutputConfig := body["output_config"]
		gt.Bool(t, hasOutputConfig).False()
	})

	t.Run("none set sends neither", func(t *testing.T) {
		body := claudeRequestBody(t, modelDef(agentkernel.ProviderClaude, "claude-sonnet-5-5"))
		_, hasThinking := body["thinking"]
		gt.Bool(t, hasThinking).False()
		_, hasOutputConfig := body["output_config"]
		gt.Bool(t, hasOutputConfig).False()
	})
}

// Both claude builders read one decision of what to send. The Anthropic API half
// is checked on the wire above; a Vertex AI client cannot be pointed at a fake
// server, so the decision is checked by value here, together with the Vertex AI
// builder adding an option for exactly the settings the decision sends.
func TestLLM_Claude_ReasoningDecisionServesBothBuilders(t *testing.T) {
	def := func(thinking, display, effort string) agentkernel.ModelDef {
		d := modelDef(agentkernel.ProviderClaude, "claude-sonnet-5-5")
		d.Thinking, d.ThinkingDisplay, d.Effort = thinking, display, effort
		return d
	}

	for name, tc := range map[string]struct {
		def          agentkernel.ModelDef
		wantAdaptive bool
		wantDisplay  string
		wantEffort   string
		wantOptions  int
	}{
		"none":                  {def("", "", ""), false, "", "", 1},
		"adaptive":              {def("adaptive", "", ""), true, "", "", 2},
		"adaptive summarized":   {def("adaptive", "summarized", ""), true, "summarized", "", 2},
		"effort only":           {def("", "", "low"), false, "", "low", 2},
		"adaptive omitted, max": {def("adaptive", "omitted", "max"), true, "omitted", "max", 3},
	} {
		t.Run(name, func(t *testing.T) {
			adaptive, display, effort := config.ClaudeReasoningForTest(tc.def)
			gt.Value(t, adaptive).Equal(tc.wantAdaptive)
			gt.String(t, display).Equal(tc.wantDisplay)
			gt.String(t, effort).Equal(tc.wantEffort)

			api, vertex := config.ClaudeOptionCountsForTest(tc.def)
			gt.Number(t, api).Equal(tc.wantOptions)
			gt.Number(t, vertex).Equal(tc.wantOptions)
		})
	}
}

// NewClientFor validates the definition, so a reasoning setting that cannot
// apply is refused even when the definition did not come from a config file.
func TestLLM_NewClientFor_RejectsReasoningOnAnotherProvider(t *testing.T) {
	cfg := config.NewLLMForTest("gpt-5", "openai-key", "", "", "")
	def := modelDef(agentkernel.ProviderOpenAI, "gpt-5")
	def.Effort = "high"

	_, err := cfg.NewClientFor(context.Background(), def)
	gt.Error(t, err).Is(agentkernel.ErrInvalidReasoningSetting)
}

func TestLLM_Gemini_RequiresProjectAndLocation(t *testing.T) {
	t.Run("missing project", func(t *testing.T) {
		cfg := config.NewLLMForTest("cheap", "", "", "", "global")
		_, err := cfg.NewClientFor(context.Background(),
			modelDef(agentkernel.ProviderGemini, "gemini-3.7-flash"))
		gt.Error(t, err)
		gt.String(t, err.Error()).Contains("gemini-project-id")
	})

	t.Run("missing location", func(t *testing.T) {
		cfg := config.NewLLMForTest("cheap", "", "", "gcp-project", "")
		_, err := cfg.NewClientFor(context.Background(),
			modelDef(agentkernel.ProviderGemini, "gemini-3.7-flash"))
		gt.Error(t, err)
		gt.String(t, err.Error()).Contains("gemini-location")
	})
}

func TestLLM_UnsupportedProvider(t *testing.T) {
	cfg := config.NewLLMForTest("bogus", "", "", "", "")

	_, err := cfg.NewClientFor(context.Background(), modelDef("bedrock", "some-model"))

	gt.Error(t, err)
	gt.String(t, err.Error()).Contains("openai, claude or gemini")
}

func TestLLM_LogAttrs_DoesNotLeakSecrets(t *testing.T) {
	cfg := config.NewLLMForTest("gpt-4o", "super-secret-key", "claude-secret", "proj", "global")
	attrs := cfg.LogAttrs()

	for _, a := range attrs {
		s := a.Value.String()
		gt.String(t, s).NotEqual("super-secret-key")
		gt.String(t, s).NotEqual("claude-secret")
	}

	found := false
	for _, a := range attrs {
		if a.Key == "model_ref" && a.Value.String() == "gpt-4o" {
			found = true
		}
	}
	gt.Bool(t, found).True()
}

func TestLLM_LogAttrs_OmitsGCPWithoutAProject(t *testing.T) {
	cfg := config.NewLLMForTest("claude-opus-5", "", "anthropic-key", "", "global")
	attrs := cfg.LogAttrs()

	for _, a := range attrs {
		gt.String(t, a.Key).NotEqual("gcp_project_id")
		gt.String(t, a.Key).NotEqual("gcp_location")
	}
}

func TestLLM_LogAttrs_IncludesGCPWhenSet(t *testing.T) {
	cfg := config.NewLLMForTest("claude-opus-5", "", "", "proj", "us-east5")
	attrs := cfg.LogAttrs()

	hasProject, hasLocation := false, false
	for _, a := range attrs {
		if a.Key == "gcp_project_id" && a.Value.String() == "proj" {
			hasProject = true
		}
		if a.Key == "gcp_location" && a.Value.String() == "us-east5" {
			hasLocation = true
		}
	}
	gt.Bool(t, hasProject).True()
	gt.Bool(t, hasLocation).True()
}
