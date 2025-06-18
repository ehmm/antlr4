// Copyright (c) 2012-2022 The ANTLR Project. All rights reserved.
// Use of this file is governed by the BSD 3-clause license that
// can be found in the LICENSE.txt file in the project root.

package antlr

import "sync"

var (
	baseRecognitionExceptionPool = sync.Pool{
		New: func() interface{} {
			return new(BaseRecognitionException)
		},
	}
)

// The root of the ANTLR exception hierarchy.
type RecognitionException interface {
	GetOffendingToken() Token
	GetOffendingState() int // Added getter
	GetMessage() string
	GetInputStream() IntStream
	GetRecognizer() Recognizer // Added getter
	GetCtx() RuleContext       // Added getter
	// error interface (Go built-in)
	Error() string
}

type BaseRecognitionException struct {
	message        string
	recognizer     Recognizer
	input          IntStream
	ctx            RuleContext
	offendingState int
	offendingToken Token // Can be nil
}

func (e *BaseRecognitionException) Reset() {
	e.message = ""
	e.recognizer = nil
	e.input = nil
	e.ctx = nil
	e.offendingState = -1
	e.offendingToken = nil
}

func releaseBaseRecognitionException(e *BaseRecognitionException) {
	if e == nil {
		return
	}
	// If specific exception types embed this and have their own Reset,
	// ensure they call this or handle all fields.
	// For now, only BaseRecognitionException itself is pooled directly.
	e.Reset()
	baseRecognitionExceptionPool.Put(e)
}

func NewBaseRecognitionException(message string, recognizer Recognizer, input IntStream, ctx RuleContext) *BaseRecognitionException {
	e := baseRecognitionExceptionPool.Get().(*BaseRecognitionException)
	e.message = message
	e.recognizer = recognizer
	e.input = input
	e.ctx = ctx
	e.offendingToken = nil // Default, can be set by specific exception types or if known
	e.offendingState = -1  // Default
	if recognizer != nil {
		e.offendingState = recognizer.GetState()
	}
	return e
}

func (e *BaseRecognitionException) GetMessage() string {
	return e.message
}

func (e *BaseRecognitionException) GetOffendingToken() Token {
	return e.offendingToken
}

func (e *BaseRecognitionException) GetOffendingState() int {
	return e.offendingState
}

func (e *BaseRecognitionException) GetInputStream() IntStream {
	return e.input
}

func (e *BaseRecognitionException) GetRecognizer() Recognizer {
	return e.recognizer
}

func (e *BaseRecognitionException) GetCtx() RuleContext {
	return e.ctx
}

// getExpectedTokens gets the set of input symbols which could potentially follow the
// previously Matched symbol at the time this exception was raised.
func (e *BaseRecognitionException) getExpectedTokens() *IntervalSet {
	if e.recognizer != nil {
		return e.recognizer.GetATN().getExpectedTokens(e.offendingState, e.ctx)
	}
	return nil
}

// Error makes BaseRecognitionException satisfy the Go error interface.
func (e *BaseRecognitionException) Error() string {
	return e.message
}

// String provides a string representation, often same as Error() or GetMessage().
func (e *BaseRecognitionException) String() string {
	return e.message
}

// LexerNoViableAltException indicates the lexer could not match any rule.
type LexerNoViableAltException struct {
	*BaseRecognitionException        // Embeds base exception
	startIndex               int    // Start index in the char stream where the error occurred
	deadEndConfigs           *ATNConfigSet // ATN configurations that led to this exception
}

func NewLexerNoViableAltException(lexer Lexer, input CharStream, startIndex int, deadEndConfigs *ATNConfigSet) *LexerNoViableAltException {
	base := NewBaseRecognitionException("LexerNoViableAltException", lexer, input, nil)
	// offendingToken is not typically set for LexerNoViableAltException directly in base,
	// as it's about failure to produce a token from startIndex.

	lnvae := &LexerNoViableAltException{
		BaseRecognitionException: base,
		startIndex:               startIndex,
		deadEndConfigs:           deadEndConfigs,
	}
	// Update message to be more specific if needed, or rely on a generic one from base.
	// For now, base message is "LexerNoViableAltException". Can be customized in lnvae.Error().
	// The original String() method built a custom message.
	// Let's ensure the message field in BaseRecognitionException is set appropriately.

	symbol := ""
	if startIndex >= 0 && startIndex < input.Size() { // Ensure input is not nil
		// Check if input is CharStream before calling GetTextFromInterval
		if cs, ok := input.(CharStream); ok {
			symbol = cs.GetTextFromInterval(NewInterval(startIndex, startIndex))
		}
	}
    // Set a more specific message
    lnvae.BaseRecognitionException.message = "LexerNoViableAltException: " + symbol


	return lnvae
}

// Error satisfies the error interface for LexerNoViableAltException.
func (e *LexerNoViableAltException) Error() string {
    // Reconstruct the more specific message if needed, or rely on base.message
    // The New function now sets a more specific message.
	return e.BaseRecognitionException.message
}

// String for LexerNoViableAltException (can be same as Error).
// The original implementation constructed a string with the symbol.
// This is now handled in NewLexerNoViableAltException by setting base.message.
// func (l *LexerNoViableAltException) String() string { ... }


// NoViableAltException indicates the parser could not find a viable alternative.
type NoViableAltException struct {
	*BaseRecognitionException
	// Specific fields for NoViableAltException
	startToken     Token // Token from which prediction began
	// offendingToken is already in BaseRecognitionException if set correctly
	deadEndConfigs *ATNConfigSet
}

func NewNoViableAltException(recognizer Parser, input TokenStream, startToken Token, offendingToken Token, deadEndConfigs *ATNConfigSet, ctx ParserRuleContext) *NoViableAltException {
	currentParserRuleContext := recognizer.GetParserRuleContext()
	if ctx != nil { // Prefer provided ctx if available
		currentParserRuleContext = ctx
	}

	actualOffendingToken := offendingToken
	if actualOffendingToken == nil && recognizer != nil {
	    actualOffendingToken = recognizer.GetCurrentToken()
	}

	base := NewBaseRecognitionException("NoViableAltException", recognizer, input, currentParserRuleContext)
	base.offendingToken = actualOffendingToken // Set the specific offending token for this exception type

	nvae := &NoViableAltException{
		BaseRecognitionException: base,
		startToken:               startToken,
		deadEndConfigs:           deadEndConfigs,
	}
    // Ensure offendingToken in the base is what was passed or determined for NVAE
    // nvae.BaseRecognitionException.offendingToken = offendingToken // already set in base
	return nvae
}

// InputMisMatchException signifies a token mismatch.
type InputMisMatchException struct {
	*BaseRecognitionException
	// No extra fields beyond what BaseRecognitionException provides for this one.
}

func NewInputMisMatchException(recognizer Parser) *InputMisMatchException {
	base := NewBaseRecognitionException("InputMisMatchException", recognizer, recognizer.GetInputStream(), recognizer.GetParserRuleContext())
	if recognizer != nil {
		base.offendingToken = recognizer.GetCurrentToken() // Specific offending token
	}
	return &InputMisMatchException{BaseRecognitionException: base}
}

// FailedPredicateException indicates a semantic predicate failed.
type FailedPredicateException struct {
	*BaseRecognitionException
	ruleIndex      int
	predicateIndex int
	predicate      string // The text of the predicate
}

func NewFailedPredicateException(recognizer Parser, predicate string, message string) *FailedPredicateException {
	finalMessage := message
	if finalMessage == "" && predicate != "" {
		finalMessage = "failed predicate: {" + predicate + "}?"
	} else if finalMessage == "" {
		finalMessage = "failed predicate"
	}

	base := NewBaseRecognitionException(finalMessage, recognizer, recognizer.GetInputStream(), recognizer.GetParserRuleContext())
	if recognizer != nil {
		base.offendingToken = recognizer.GetCurrentToken() // Token at which predicate failed
	}

	fpe := &FailedPredicateException{
		BaseRecognitionException: base,
		predicate:                predicate,
		// ruleIndex and predicateIndex need to be determined from ATN state
	}

	if recognizer != nil {
		s := recognizer.GetInterpreter().atn.states[recognizer.GetState()]
		// Ensure s and its transitions are valid before accessing
		if s != nil && len(s.GetTransitions()) > 0 {
			trans := s.GetTransitions()[0] // Assuming the relevant transition is the first one
			if pt, ok := trans.(*PredicateTransition); ok {
				fpe.ruleIndex = pt.ruleIndex
				fpe.predicateIndex = pt.predIndex
			} else {
				fpe.ruleIndex = 0      // Default or sentinel
				fpe.predicateIndex = 0 // Default or sentinel
			}
		}
	}
	return fpe
}

// formatMessage helper is now integrated into NewFailedPredicateException.
// func (f *FailedPredicateException) formatMessage(predicate, message string) string { ... }

// ParseCancellationException is a special exception type not typically pooled with others.
type ParseCancellationException struct {
	// No fields, it's a marker type often. If it needs to carry info, add fields.
	// It does not embed BaseRecognitionException, so it won't use the pool.
}

// GetOffendingToken makes ParseCancellationException satisfy RecognitionException (partially, if needed)
// However, it's often used to signal cancellation without detailed error info.
func (p *ParseCancellationException) GetOffendingToken() Token { return nil }
func (p *ParseCancellationException) GetOffendingState() int   { return -1 }
func (p *ParseCancellationException) GetMessage() string       { return "ParseCancellationException" }
func (p *ParseCancellationException) GetInputStream() IntStream{ return nil }
func (p *ParseCancellationException) GetRecognizer() Recognizer{ return nil }
func (p *ParseCancellationException) GetCtx() RuleContext      { return nil }
func (p *ParseCancellationException) Error() string            { return "ParseCancellationException" }


func NewParseCancellationException() *ParseCancellationException {
	return new(ParseCancellationException) // Not pooled
}

[end of runtime/Go/antlr/v4/errors.go]
