package image

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"

	kargoapi "github.com/akuity/kargo/api/v1alpha1"
	"github.com/akuity/kargo/pkg/logging"
)

func init() {
	defaultSelectorRegistry.MustRegister(
		selectorRegistration{
			Predicate: func(_ context.Context, sub kargoapi.ImageSubscription) (bool, error) {
				return sub.ImageSelectionStrategy == kargoapi.ImageSelectionStrategyLexical, nil
			},
			Value: newLexicalSelector,
		},
	)
}

// lexicalSelector implements the Selector interface for
// kargoapi.ImageSelectionStrategyLexical.
type lexicalSelector struct {
	*tagBasedSelector
}

func newLexicalSelector(
	sub kargoapi.ImageSubscription,
	creds *Credentials,
) (Selector, error) {
	tagBased, err := newTagBasedSelector(sub, creds)
	if err != nil {
		return nil, fmt.Errorf("error building tag based selector: %w", err)
	}
	return &lexicalSelector{tagBasedSelector: tagBased}, nil
}

// Select implements the Selector interface.
func (l *lexicalSelector) Select(
	ctx context.Context,
) ([]kargoapi.DiscoveredImageReference, error) {
	loggerCtx := append(
		l.getLoggerContext(),
		"selectionStrategy", kargoapi.ImageSelectionStrategyLexical,
	)
	logger := logging.LoggerFromContext(ctx).WithValues(loggerCtx...)
	ctx = logging.ContextWithLogger(ctx, logger)

	logger.Trace("discovering images")

	tags, err := l.repoClient.getTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("error listing tags: %w", err)
	}
	if len(tags) == 0 {
		logger.Trace("found no tags")
		return nil, nil
	}
	logger.Trace("got all tags")

	tags = l.filterTags(tags)
	if len(tags) == 0 {
		logger.Trace("no tags matched criteria")
		return nil, nil
	}
	logger.Trace(
		"tags matched initial criteria",
		"count", len(tags),
	)

	logger.Trace("sorting tags lexically")
	tags = l.sortTags(tags)

	images, err := l.getImagesByTags(ctx, tags)
	if err != nil {
		return nil, fmt.Errorf("error getting images by tags: %w", err)
	}

	if len(images) == 0 {
		logger.Trace("no images matched criteria")
		return nil, nil
	}

	logger.Trace(
		"discovered images",
		"count", len(images),
	)

	return l.imagesToAPIImages(images, l.discoveryLimit), nil
}

// sortTags sorts the provided tags using natural/numeric ordering, returning
// them sorted from greatest to least. This ensures that tags like "2024.12.1"
// are correctly sorted as newer than "2024.2.1", which would not be the case
// with pure lexical (string) sorting.
func (l *lexicalSelector) sortTags(tags []string) []string {
	sorted := make([]string, len(tags))
	copy(sorted, tags)

	slices.SortFunc(sorted, func(a, b string) int {
		return -compareNatural(a, b) // Negative for descending order
	})

	return sorted
}

// compareNatural performs a natural/numeric comparison of two strings.
// Returns:
//   - negative if a < b
//   - zero if a == b
//   - positive if a > b
//
// This function splits strings into alternating sequences of non-numeric and
// numeric characters. Numeric sequences are compared as numbers, while
// non-numeric sequences are compared lexically.
func compareNatural(a, b string) int {
	aTokens := tokenize(a)
	bTokens := tokenize(b)

	for i := 0; i < len(aTokens) && i < len(bTokens); i++ {
		aToken := aTokens[i]
		bToken := bTokens[i]

		// If both tokens are numeric, compare them as numbers
		if aToken.isNumeric && bToken.isNumeric {
			if cmp := compareNumeric(aToken.value, bToken.value); cmp != 0 {
				return cmp
			}
			continue
		}

		// If only one is numeric, numeric comes after non-numeric
		if aToken.isNumeric != bToken.isNumeric {
			if aToken.isNumeric {
				return 1 // a is numeric, b is not, so a > b
			}
			return -1 // b is numeric, a is not, so a < b
		}

		// Both are non-numeric, compare lexically
		if cmp := strings.Compare(aToken.value, bToken.value); cmp != 0 {
			return cmp
		}
	}

	// If all compared tokens are equal, the longer string is greater
	return len(aTokens) - len(bTokens)
}

// token represents a portion of a string that is either numeric or non-numeric.
type token struct {
	value     string
	isNumeric bool
}

// tokenize splits a string into alternating sequences of numeric and
// non-numeric characters.
func tokenize(s string) []token {
	if len(s) == 0 {
		return nil
	}

	var tokens []token
	var currentToken strings.Builder
	// Determine if first character is numeric
	isNumeric := unicode.IsDigit(rune(s[0]))

	for _, ch := range s {
		chIsNumeric := unicode.IsDigit(ch)

		if chIsNumeric == isNumeric {
			// Continue current token
			currentToken.WriteRune(ch)
		} else {
			// Save current token and start new one
			if currentToken.Len() > 0 {
				tokens = append(tokens, token{
					value:     currentToken.String(),
					isNumeric: isNumeric,
				})
				currentToken.Reset()
			}
			currentToken.WriteRune(ch)
			isNumeric = chIsNumeric
		}
	}

	// Add final token
	if currentToken.Len() > 0 {
		tokens = append(tokens, token{
			value:     currentToken.String(),
			isNumeric: isNumeric,
		})
	}

	return tokens
}

// compareNumeric compares two numeric strings as integers.
// This function assumes that both strings contain only digits (as ensured by the tokenizer).
// Returns:
//   - negative if a < b
//   - zero if a == b
//   - positive if a > b
func compareNumeric(a, b string) int {
	// Try parsing as uint64 for better range (0 to 2^64-1)
	aNum, aErr := strconv.ParseUint(a, 10, 64)
	bNum, bErr := strconv.ParseUint(b, 10, 64)

	// If both parse successfully, compare numerically
	if aErr == nil && bErr == nil {
		if aNum < bNum {
			return -1
		}
		if aNum > bNum {
			return 1
		}
		// If numerically equal, compare string length (e.g., "01" vs "1")
		// Longer strings (with leading zeros) should come after shorter ones
		if len(a) != len(b) {
			return len(a) - len(b)
		}
		return 0
	}

	// If numbers are too large for uint64, compare by length first (more digits = larger)
	// then lexically if same length. This handles version numbers with huge components.
	if aErr != nil && bErr != nil {
		if len(a) != len(b) {
			return len(a) - len(b)
		}
		return strings.Compare(a, b)
	}

	// If only one parses, the one that doesn't parse is too large (more digits)
	if aErr != nil {
		return 1 // a is too large, so a > b
	}
	return -1 // b is too large, so a < b
}
