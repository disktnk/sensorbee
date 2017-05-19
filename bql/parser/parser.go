package parser

//go:generate peg bql.peg

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
)

// BQLParser parsing BQL syntaxes represented by PEG.
type BQLParser struct {
	b bqlPeg
}

// New returns a parser for BQL.
func New() *BQLParser {
	return &BQLParser{}
}

// ParseStmt returns a parsed node and rest string.
func (p *BQLParser) ParseStmt(s string) (result interface{}, rest string, err error) {
	// catch any parser errors
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Error in BQL parser: %v", r)
		}
	}()
	// parse the statement
	b := p.b
	b.Buffer = s
	b.Init()
	if err := b.Parse(); err != nil {
		return nil, "", err
	}
	b.Execute()
	if b.parseStack.Peek() == nil {
		// the statement was parsed ok, but not put on the stack?
		// this should never occur.
		return nil, "", fmt.Errorf("no valid BQL statement could be parsed")
	}
	stackElem := b.parseStack.Pop()
	// we look at the part of the string right of the parsed
	// statement. note that we expect that trailing whitespace
	// or comments are already included in the range [0:stackElem.end]
	// as done by IncludeTrailingWhitespace() so that we do not
	// return a comment-only string as rest.
	isSpaceOrSemicolon := func(r rune) bool {
		return unicode.IsSpace(r) || r == rune(';')
	}
	rest = strings.TrimLeftFunc(string([]rune(s)[stackElem.end:]), isSpaceOrSemicolon)
	// pop it from the parse stack
	return stackElem.comp, rest, nil
}

// ParseStmts returns statement node.
func (p *BQLParser) ParseStmts(s string) ([]interface{}, error) {
	// parse all statements
	results := []interface{}{}
	rest := strings.TrimSpace(s)
	for rest != "" {
		result, restTmp, err := p.ParseStmt(rest)
		if err != nil {
			return nil, err
		}
		// append the parsed statement to the result list
		results = append(results, result)
		rest = restTmp
	}
	return results, nil
}

type bqlPeg struct {
	bqlPegBackend
}

func (b *bqlPeg) Parse(rule ...int) error {
	// override the Parse method from the bqlPegBackend in order
	// to place our own error before returning
	if err := b.bqlPegBackend.Parse(rule...); err != nil {
		if pErr, ok := err.(*parseError); ok {
			return &bqlParseError{pErr}
		}
		return err
	}
	return nil
}

type bqlParseError struct {
	*parseError
}

func (e *bqlParseError) Error() string {
	error := "failed to parse string as BQL statement\n"
	stmt := []rune(e.p.Buffer)
	// now find the offensive line
	foundError := false
	for _, token := range e.p.Tokens() {
		begin, end := int(token.begin), int(token.end)
		if end == 0 {
			// these are '' matches we cannot exploit for a useful error message
			continue
		} else if foundError {
			// if we found an error, the next tokens may give some additional
			// information about what kind of statement we have here. the first
			// rule that starts at 0 is (often?) the description we want.
			ruleName := rul3s[token.pegRule]
			if begin == 0 && end > 0 {
				error += fmt.Sprintf("\nconsider to look up the documentation for %s",
					ruleName)
				break
			}
		} else if end > 0 {
			// collect the max token in error and translate their
			// string indexes into line/symbol pairs
			end = int(e.max.end)
			positions := []int{int(e.max.begin), end}
			translations := translatePositions(e.p.buffer, positions)
			error += fmt.Sprintf("statement has a syntax error near line %v, symbol %v:\n",
				translations[end].line, translations[end].symbol)
			// we want some output like:
			//
			//   ... FROM x [RANGE 7 UPLES] WHERE ...
			//                       ^
			//
			snipStartIdx := end - 20
			snipStart := "..."
			if snipStartIdx < 0 {
				snipStartIdx = 0
				snipStart = ""
			}
			snipEndIdx := end + 30
			snipEnd := "..."
			if snipEndIdx > len(stmt) {
				snipEndIdx = len(stmt)
				snipEnd = ""
			}
			// first line: an excerpt from the statement
			error += "  " + snipStart
			snipBeforeErr := strings.Replace(string(stmt[snipStartIdx:end]), "\n", " ", -1)
			snipAfterInclErr := strings.Replace(string(stmt[end:snipEndIdx]), "\n", " ", -1)
			error += snipBeforeErr + snipAfterInclErr
			error += snipEnd + "\n"
			// second line: a ^ marker at the correct position
			error += strings.Repeat(" ", len(snipStart)+2)
			error += strings.Repeat(" ", runewidth.StringWidth(snipBeforeErr))
			error += "^"
			foundError = true
		}
	}
	if !foundError {
		error += "statement has an unlocatable syntax error"
	}

	return error
}
