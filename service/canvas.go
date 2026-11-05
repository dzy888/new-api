package service

import (
	"cmp"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// CanvasModel is a public model offer. Credentials and channel identities never leave the gateway.
type CanvasModel struct {
	Name         string   `json:"name"`
	Capabilities []string `json:"capabilities"`
	APIFormat    string   `json:"api_format"`
	TextEndpoint string   `json:"text_endpoint"`
	Group        string   `json:"group"`
	GroupRatio   float64  `json:"group_ratio"`
	Description  string   `json:"description,omitempty"`
}

func GetCanvasModels(userGroup string) []CanvasModel {
	usable := GetUserUsableGroups(userGroup)
	groups := make([]string, 0, len(usable))
	for group := range usable {
		if group != "auto" && ratio_setting.ContainsGroupRatio(group) {
			groups = append(groups, group)
		}
	}
	slices.Sort(groups)
	// Existing administrator-defined Auto order breaks ties after the user's own group.
	priority := append([]string{userGroup}, GetUserAutoGroup(userGroup)...)
	for _, group := range groups {
		if !slices.Contains(priority, group) {
			priority = append(priority, group)
		}
	}
	return BuildCanvasModels(userGroup, priority, usable, model.GetPricing())
}

// BuildCanvasModels chooses one stable, authorized route per model; it never chooses by price.
func BuildCanvasModels(userGroup string, priority []string, usable map[string]string, prices []model.Pricing) []CanvasModel {
	result := make([]CanvasModel, 0)
	for _, price := range prices {
		group := ""
		for _, candidate := range priority {
			if candidate == "auto" {
				continue
			}
			if _, allowed := usable[candidate]; !allowed {
				continue
			}
			if slices.Contains(price.EnableGroup, candidate) || slices.Contains(price.EnableGroup, "all") {
				group = candidate
				break
			}
		}
		if group == "" {
			continue
		}
		offer := CanvasModel{Name: price.ModelName, APIFormat: "openai", TextEndpoint: "chat", Group: group,
			GroupRatio: GetUserGroupRatio(userGroup, group), Description: price.Description}
		endpoints := price.SupportedEndpointTypes
		switch {
		case strings.Contains(strings.ToLower(price.ModelName), "tts") || strings.Contains(strings.ToLower(price.ModelName), "speech"):
			offer.Capabilities = []string{"audio"}
		case slices.Contains(endpoints, constant.EndpointTypeOpenAIVideo):
			offer.Capabilities = []string{"video"}
		case slices.Contains(endpoints, constant.EndpointTypeImageGeneration):
			offer.Capabilities = []string{"image"}
		default:
			// Prefer supported host protocols. Unsupported embedding/rerank models are omitted.
			if slices.Contains(endpoints, constant.EndpointTypeOpenAIResponse) {
				offer.Capabilities = []string{"text"}
				offer.TextEndpoint = "responses"
			} else if slices.Contains(endpoints, constant.EndpointTypeOpenAI) || slices.Contains(endpoints, constant.EndpointTypeAnthropic) {
				offer.Capabilities = []string{"text"}
			} else if slices.Contains(endpoints, constant.EndpointTypeGemini) {
				offer.Capabilities = []string{"text"}
				offer.APIFormat = "gemini"
			}
		}
		if slices.Contains(endpoints, constant.EndpointTypeGemini) && (strings.Contains(strings.ToLower(price.ModelName), "image") || strings.Contains(strings.ToLower(price.ModelName), "nano-banana")) {
			offer.Capabilities = []string{"image", "text"}
			offer.APIFormat = "gemini"
		}
		if len(offer.Capabilities) > 0 {
			result = append(result, offer)
		}
	}
	slices.SortFunc(result, func(a, b CanvasModel) int { return cmp.Compare(a.Name, b.Name) })
	return result
}
