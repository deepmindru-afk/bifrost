package schemas

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/bytedance/sonic"
)

// DefaultPageSize is the default page size for listing models
const DefaultPageSize = 1000

// MaxPaginationRequests is the maximum number of pagination requests to make
const MaxPaginationRequests = 20

// Structure to collect results from goroutines
type ListModelsByKeyResult struct {
	Response *BifrostListModelsResponse
	Err      *BifrostError
	KeyID    string
}

type BifrostListModelsRequest struct {
	Provider ModelProvider `json:"provider"`

	PageSize int `json:"page_size"`

	// PageToken: Token received from previous request to retrieve next page
	PageToken string `json:"page_token"`

	// ExtraParams: Additional provider-specific query parameters
	// This allows for flexibility to pass any custom parameters that specific providers might support
	ExtraParams map[string]interface{} `json:"-"`
}

type BifrostListModelsResponse struct {
	Data          []Model                    `json:"data"`
	ExtraFields   BifrostResponseExtraFields `json:"extra_fields"`
	NextPageToken string                     `json:"next_page_token,omitempty"` // Token to retrieve next page

	// Anthropic specific fields
	FirstID *string `json:"-"`
	LastID  *string `json:"-"`
	HasMore *bool   `json:"-"`
}

// ApplyPagination applies offset-based pagination to a BifrostListModelsResponse.
// Uses opaque tokens with LastID validation to ensure cursor integrity.
// Returns the paginated response with properly set NextPageToken.
func (response *BifrostListModelsResponse) ApplyPagination(pageSize int, pageToken string) *BifrostListModelsResponse {
	if response == nil {
		return nil
	}

	totalItems := len(response.Data)

	if pageSize <= 0 {
		return response
	}

	cursor := decodePaginationCursor(pageToken)
	offset := cursor.Offset

	// Validate cursor integrity if LastID is present
	if cursor.LastID != "" && !validatePaginationCursor(cursor, response.Data) {
		// Invalid cursor: reset to beginning
		offset = 0
	}

	if offset >= totalItems {
		// Return empty page, no next token
		return &BifrostListModelsResponse{
			Data:          []Model{},
			ExtraFields:   response.ExtraFields,
			NextPageToken: "",
		}
	}

	endIndex := offset + pageSize
	if endIndex > totalItems {
		endIndex = totalItems
	}

	paginatedData := response.Data[offset:endIndex]

	paginatedResponse := &BifrostListModelsResponse{
		Data:        paginatedData,
		ExtraFields: response.ExtraFields,
	}

	if endIndex < totalItems {
		// Get the last item ID for cursor validation
		var lastID string
		if len(paginatedData) > 0 {
			lastID = paginatedData[len(paginatedData)-1].ID
		}

		nextToken, err := encodePaginationCursor(endIndex, lastID)
		if err == nil {
			paginatedResponse.NextPageToken = nextToken
		}
	} else {
		paginatedResponse.NextPageToken = ""
	}

	return paginatedResponse
}

type PricingFetcher func(model string, provider ModelProvider) *DataSheetPricingEntry

// AddPricing adds pricing data to the response.
// This is used to add pricing data to the response.
//
// Parameters:
//   - fetcher: The pricing fetcher function
//
// Returns:
//   - response: The response with pricing data
func (response *BifrostListModelsResponse) AddPricing(fetcher PricingFetcher) {
	for i, modelData := range response.Data {
		model := strings.TrimPrefix(modelData.ID, string(response.ExtraFields.Provider)+"/")
		pricing := fetcher(model, response.ExtraFields.Provider)
		if pricing != nil {
			if response.Data[i].Pricing == nil {
				response.Data[i].Pricing = &Pricing{}
			}
			response.Data[i].Pricing.DataSheetPricingEntry = pricing
		}
	}
}

type Model struct {
	ID                  string             `json:"id"`
	CanonicalSlug       *string            `json:"canonical_slug,omitempty"`
	DeploymentName      *string            `json:"deployment_name,omitempty"`
	Name                *string            `json:"name,omitempty"`
	Created             *int64             `json:"created,omitempty"`
	ContextLength       *int               `json:"context_length,omitempty"`
	MaxInputTokens      *int               `json:"max_input_tokens,omitempty"`
	MaxOutputTokens     *int               `json:"max_output_tokens,omitempty"`
	Architecture        *Architecture      `json:"architecture,omitempty"`
	Pricing             *Pricing           `json:"pricing,omitempty"`
	TopProvider         *TopProvider       `json:"top_provider,omitempty"`
	PerRequestLimits    *PerRequestLimits  `json:"per_request_limits,omitempty"`
	SupportedParameters []string           `json:"supported_parameters,omitempty"`
	DefaultParameters   *DefaultParameters `json:"default_parameters,omitempty"`
	HuggingFaceID       *string            `json:"hugging_face_id,omitempty"`
	Description         *string            `json:"description,omitempty"`

	OwnedBy          *string  `json:"owned_by,omitempty"`
	SupportedMethods []string `json:"supported_methods,omitempty"`
}

type Architecture struct {
	Modality         *string  `json:"modality,omitempty"`
	Tokenizer        *string  `json:"tokenizer,omitempty"`
	InstructType     *string  `json:"instruct_type,omitempty"`
	InputModalities  []string `json:"input_modalities,omitempty"`
	OutputModalities []string `json:"output_modalities,omitempty"`
}

type Pricing struct {
	Prompt            *string `json:"prompt,omitempty"`
	Completion        *string `json:"completion,omitempty"`
	Request           *string `json:"request,omitempty"`
	Image             *string `json:"image,omitempty"`
	WebSearch         *string `json:"web_search,omitempty"`
	InternalReasoning *string `json:"internal_reasoning,omitempty"`
	InputCacheRead    *string `json:"input_cache_read,omitempty"`
	InputCacheWrite   *string `json:"input_cache_write,omitempty"`

	*DataSheetPricingEntry
}

type TopProvider struct {
	IsModerated         *bool `json:"is_moderated,omitempty"`
	ContextLength       *int  `json:"context_length,omitempty"`
	MaxCompletionTokens *int  `json:"max_completion_tokens,omitempty"`
}

type PerRequestLimits struct {
	PromptTokens     *int `json:"prompt_tokens,omitempty"`
	CompletionTokens *int `json:"completion_tokens,omitempty"`
}

type DefaultParameters struct {
	Temperature      *float64 `json:"temperature,omitempty"`
	TopP             *float64 `json:"top_p,omitempty"`
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`
}

// paginationCursor represents the internal cursor structure for pagination.
type paginationCursor struct {
	Offset int    `json:"o"`
	LastID string `json:"l,omitempty"`
}

// PricingEntry represents a single model's pricing information
type DataSheetPricingEntry struct {
	// Basic pricing
	InputCostPerToken  float64 `json:"input_cost_per_token"`
	OutputCostPerToken float64 `json:"output_cost_per_token"`
	Provider           string  `json:"provider"`
	Mode               string  `json:"mode"`

	// Additional pricing for media
	InputCostPerImage          *float64 `json:"input_cost_per_image,omitempty"`
	InputCostPerVideoPerSecond *float64 `json:"input_cost_per_video_per_second,omitempty"`
	InputCostPerAudioPerSecond *float64 `json:"input_cost_per_audio_per_second,omitempty"`

	// Character-based pricing
	InputCostPerCharacter  *float64 `json:"input_cost_per_character,omitempty"`
	OutputCostPerCharacter *float64 `json:"output_cost_per_character,omitempty"`

	// Pricing above 128k tokens
	InputCostPerTokenAbove128kTokens          *float64 `json:"input_cost_per_token_above_128k_tokens,omitempty"`
	InputCostPerCharacterAbove128kTokens      *float64 `json:"input_cost_per_character_above_128k_tokens,omitempty"`
	InputCostPerImageAbove128kTokens          *float64 `json:"input_cost_per_image_above_128k_tokens,omitempty"`
	InputCostPerVideoPerSecondAbove128kTokens *float64 `json:"input_cost_per_video_per_second_above_128k_tokens,omitempty"`
	InputCostPerAudioPerSecondAbove128kTokens *float64 `json:"input_cost_per_audio_per_second_above_128k_tokens,omitempty"`
	OutputCostPerTokenAbove128kTokens         *float64 `json:"output_cost_per_token_above_128k_tokens,omitempty"`
	OutputCostPerCharacterAbove128kTokens     *float64 `json:"output_cost_per_character_above_128k_tokens,omitempty"`

	// Cache and batch pricing
	CacheReadInputTokenCost   *float64 `json:"cache_read_input_token_cost,omitempty"`
	InputCostPerTokenBatches  *float64 `json:"input_cost_per_token_batches,omitempty"`
	OutputCostPerTokenBatches *float64 `json:"output_cost_per_token_batches,omitempty"`
}

// encodePaginationCursor creates an opaque base64-encoded page token from cursor data.
// Returns empty string if offset is 0 or negative.
func encodePaginationCursor(offset int, lastID string) (string, error) {
	if offset <= 0 {
		return "", nil
	}

	cursor := paginationCursor{
		Offset: offset,
		LastID: lastID,
	}

	jsonData, err := sonic.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("failed to marshal pagination cursor: %w", err)
	}

	// Use URL-safe base64 encoding without padding for opaque token
	encoded := base64.RawURLEncoding.EncodeToString(jsonData)
	return encoded, nil
}

// decodePaginationCursor extracts cursor data from an opaque base64-encoded page token.
// Returns cursor with 0 offset for empty or invalid tokens.
func decodePaginationCursor(token string) paginationCursor {
	if token == "" {
		return paginationCursor{}
	}

	// Decode base64
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return paginationCursor{}
	}

	var cursor paginationCursor
	if err := sonic.Unmarshal(decoded, &cursor); err != nil {
		return paginationCursor{}
	}

	if cursor.Offset < 0 {
		return paginationCursor{}
	}

	return cursor
}

// validatePaginationCursor validates that the cursor matches the expected position in the data.
// Returns true if the cursor is valid, false otherwise.
func validatePaginationCursor(cursor paginationCursor, data []Model) bool {
	if cursor.LastID == "" {
		return true
	}

	if cursor.Offset <= 0 || cursor.Offset > len(data) {
		return false
	}

	prevIndex := cursor.Offset - 1
	if prevIndex >= 0 && prevIndex < len(data) {
		return data[prevIndex].ID == cursor.LastID
	}

	return true
}
