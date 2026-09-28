package parserfilter

import "lagoon.sh/insights-remote/internal"

// a parserFilter will
type parserFilter interface {
	isFilteredOut() bool
	hasError() bool
	getError() error
	isOfType(typename string) parserFilter
	fieldContains(fieldname string, regex string) parserFilter
	fieldContainsExactMatch(fieldname string, match string) parserFilter
	setKeyFact(isKeyFact bool) parserFilter
	setFactField(fieldname string, value string) parserFilter
	getFact() internal.Fact
}
