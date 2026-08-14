package recommendation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"go.opentelemetry.io/otel/attribute"

	"spacetime-node/internal/platform/observability"
)

const DefaultCopyTimeout = 250 * time.Millisecond

var (
	ErrInvalidCopyFacts  = errors.New("invalid copy facts")
	ErrInvalidCopyOutput = errors.New("invalid copy output")
	ErrCopyGeneration    = errors.New("copy generation failed")
)

type CopyFacts struct {
	StationName         string   `json:"station_name"`
	OfferName           string   `json:"offer_name"`
	PointsCost          int64    `json:"points_cost"`
	DiscountDescription string   `json:"discount_description"`
	ReasonLabels        []string `json:"reason_labels"`
	TimeContext         string   `json:"time_context,omitempty"`
}

type CopyOutput struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Tone  string `json:"tone"`
}

type CopyGenerator func(context.Context, CopyFacts) (CopyOutput, error)

func TemplateCopy(facts CopyFacts) (CopyOutput, error) {
	if err := ValidateCopyFacts(facts); err != nil {
		return CopyOutput{}, err
	}
	body := fmt.Sprintf("%s：%s，使用 %d 點即可。推薦理由：%s", truncateRunes(facts.StationName, 20), truncateRunes(facts.DiscountDescription, 80), facts.PointsCost, truncateRunes(strings.Join(facts.ReasonLabels, "、"), 30))
	if facts.TimeContext != "" {
		body += "，適合" + facts.TimeContext + "。"
	}
	output := CopyOutput{
		Title: truncateRunes(facts.OfferName, 40),
		Body:  truncateRunes(body, 140),
		Tone:  "friendly",
	}
	if err := ValidateCopyOutput(facts, output); err != nil {
		return CopyOutput{}, err
	}
	return output, nil
}

func GenerateCopy(ctx context.Context, facts CopyFacts, generator CopyGenerator, timeout time.Duration) (CopyOutput, string, error) {
	template, err := TemplateCopy(facts)
	if err != nil {
		return CopyOutput{}, "", err
	}
	if generator == nil {
		observability.AddCounter(ctx, "copy_generation_total", 1, attribute.String("copy.source", "template"), attribute.String("copy.outcome", "template"))
		return template, "template", nil
	}
	if timeout <= 0 {
		timeout = DefaultCopyTimeout
	}
	copyCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	output, err := generator(copyCtx, facts)
	if err == nil {
		err = ValidateCopyOutput(facts, output)
	}
	if err != nil {
		outcome := "llm_error"
		if copyCtx.Err() == context.DeadlineExceeded {
			outcome = "timeout"
		} else if errors.Is(err, ErrInvalidCopyOutput) {
			outcome = "validation_failure"
		}
		observability.AddCounter(ctx, "copy_generation_total", 1, attribute.String("copy.source", "template"), attribute.String("copy.outcome", outcome))
		return template, "template", nil
	}
	observability.AddCounter(ctx, "copy_generation_total", 1, attribute.String("copy.source", "llm"), attribute.String("copy.outcome", "success"))
	return output, "llm", nil
}

func ValidateCopyFacts(facts CopyFacts) error {
	if facts.StationName == "" || runeCount(facts.StationName) > 80 || facts.OfferName == "" || runeCount(facts.OfferName) > 120 || facts.PointsCost < 0 || facts.DiscountDescription == "" || runeCount(facts.DiscountDescription) > 160 || len(facts.ReasonLabels) < 1 || len(facts.ReasonLabels) > 3 || runeCount(facts.TimeContext) > 40 {
		return ErrInvalidCopyFacts
	}
	for _, label := range facts.ReasonLabels {
		if label == "" || runeCount(label) > 40 {
			return ErrInvalidCopyFacts
		}
	}
	return nil
}

func ValidateCopyOutput(facts CopyFacts, output CopyOutput) error {
	if ValidateCopyFacts(facts) != nil || output.Title == "" || runeCount(output.Title) > 40 || output.Body == "" || runeCount(output.Body) > 140 || (output.Tone != "friendly" && output.Tone != "calm" && output.Tone != "urgent") {
		return ErrInvalidCopyOutput
	}
	text := output.Title + " " + output.Body
	if !strings.Contains(text, factToken(facts.OfferName)) || !strings.Contains(text, strconv.FormatInt(facts.PointsCost, 10)) || !strings.Contains(text, factToken(facts.DiscountDescription)) {
		return ErrInvalidCopyOutput
	}
	allowedNumbers := numericTokens(facts.OfferName + " " + facts.DiscountDescription + " " + strconv.FormatInt(facts.PointsCost, 10))
	for number := range numericTokens(text) {
		if !allowedNumbers[number] {
			return ErrInvalidCopyOutput
		}
	}
	return nil
}

func NewHTTPCopyGenerator(baseURL, model string, client *http.Client) CopyGenerator {
	if client == nil {
		client = &http.Client{Timeout: DefaultCopyTimeout}
	}
	return func(ctx context.Context, facts CopyFacts) (CopyOutput, error) {
		if baseURL == "" || model == "" {
			return CopyOutput{}, ErrCopyGeneration
		}
		factsJSON, err := json.Marshal(facts)
		if err != nil {
			return CopyOutput{}, err
		}
		requestBody, err := json.Marshal(struct {
			Model       string  `json:"model"`
			Temperature float32 `json:"temperature"`
			Messages    []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}{
			Model:       model,
			Temperature: 0,
			Messages: []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			}{
				{Role: "system", Content: "只輸出符合 schema 的 JSON 文案，不得改寫 facts。"},
				{Role: "user", Content: string(factsJSON)},
			},
		})
		if err != nil {
			return CopyOutput{}, err
		}
		endpoint, err := url.JoinPath(strings.TrimRight(baseURL, "/"), "v1/chat/completions")
		if err != nil {
			return CopyOutput{}, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(requestBody))
		if err != nil {
			return CopyOutput{}, err
		}
		req.Header.Set("Content-Type", "application/json")
		response, err := client.Do(req)
		if err != nil {
			return CopyOutput{}, err
		}
		defer response.Body.Close()
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
			return CopyOutput{}, fmt.Errorf("%w: status %s", ErrCopyGeneration, response.Status)
		}
		var completion struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&completion); err != nil || len(completion.Choices) == 0 {
			return CopyOutput{}, ErrCopyGeneration
		}
		var output CopyOutput
		decoder := json.NewDecoder(strings.NewReader(completion.Choices[0].Message.Content))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&output); err != nil {
			return CopyOutput{}, ErrCopyGeneration
		}
		return output, nil
	}
}

func factToken(value string) string {
	runes := []rune(value)
	if len(runes) > 20 {
		runes = runes[:20]
	}
	return string(runes)
}

func numericTokens(value string) map[string]bool {
	tokens := make(map[string]bool)
	var current []rune
	flush := func() {
		if len(current) > 0 {
			tokens[string(current)] = true
			current = nil
		}
	}
	for _, character := range value {
		if unicode.IsDigit(character) {
			current = append(current, character)
			continue
		}
		flush()
	}
	flush()
	return tokens
}

func runeCount(value string) int { return len([]rune(value)) }

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
