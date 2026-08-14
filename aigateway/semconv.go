package aigateway

import "strings"

// Span attribute names for generative AI calls.
//
// Defined here rather than taken from a semconv module on purpose. The GenAI conventions are still
// marked Development upstream and moved repositories in 2026, and more immediately: these names are
// chosen to match what Cloudflare's AI Gateway actually emits, which is not what Cloudflare's own
// documentation says. Their live spans use gen_ai.provider.name where the docs promise
// gen_ai.model.provider. Since both sets of spans end up in the same backend, a query should not need
// to know which half it is looking at.
const (
	attrOperationName = "gen_ai.operation.name"
	attrProviderName  = "gen_ai.provider.name"
	attrRequestModel  = "gen_ai.request.model"
	attrResponseModel = "gen_ai.response.model"
	attrInputTokens   = "gen_ai.usage.input_tokens"
	attrOutputTokens  = "gen_ai.usage.output_tokens"
	attrFinishReason  = "gen_ai.response.finish_reasons"

	// Ours, namespaced because they have no upstream equivalent. Cloudflare reports cached tokens
	// only inside a JSON blob rather than as an attribute, so this is additive rather than duplicate.
	attrCachedInputTokens = "siikabot.gen_ai.cached_input_tokens"
	attrAttempt           = "siikabot.gen_ai.attempt"
)

// operationChat is the only GenAI operation this bot performs
const operationChat = "chat"

// providerFromModel derives the provider from a model id such as "openai/gpt-4o-mini".
//
// Cloudflare reports the provider as a bare name, so the prefix is split off to match rather than
// sending the full model id and forcing every query to account for two formats.
func providerFromModel(model string) string {
	if provider, _, found := strings.Cut(model, "/"); found {
		return provider
	}
	return model
}
