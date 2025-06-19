// Copyright (c) 2012-2022 The ANTLR Project. All rights reserved.
// Use of this file is governed by the BSD 3-clause license that
// can be found in the LICENSE.txt file in the project root.

package antlr

import (
	"strconv"
	"strings"
	"sync"
)

type TokenSourceCharStreamPair struct {
	tokenSource TokenSource
	charStream  CharStream
}

// A token has properties: text, type, line, character position in the line
// (so we can ignore tabs), token channel, index, and source from which
// we obtained this token.

type Token interface {
	GetSource() *TokenSourceCharStreamPair
	GetTokenType() int
	GetChannel() int
	GetStart() int
	GetStop() int
	GetLine() int
	GetColumn() int

	GetText() string
	SetText(s string)

	GetTokenIndex() int
	SetTokenIndex(v int)

	GetTokenSource() TokenSource
	GetInputStream() CharStream

	String() string
}

type BaseToken struct {
	source     *TokenSourceCharStreamPair
	tokenType  int    // token type of the token
	channel    int    // The parser ignores everything not on DEFAULT_CHANNEL
	start      int    // optional return -1 if not implemented.
	stop       int    // optional return -1 if not implemented.
	tokenIndex int    // from 0..n-1 of the token object in the input stream
	line       int    // line=1..n of the 1st character
	column     int    // beginning of the line at which it occurs, 0..n-1
	text       string // text of the token.
	readOnly   bool
}

const (
	TokenInvalidType = 0

	// TokenEpsilon  - during lookahead operations, this "token" signifies we hit the rule end [ATN] state
	// and did not follow it despite needing to.
	TokenEpsilon = -2

	TokenMinUserTokenType = 1

	TokenEOF = -1

	// TokenDefaultChannel is the default channel upon which tokens are sent to the parser.
	//
	// All tokens go to the parser (unless [Skip] is called in the lexer rule)
	// on a particular "channel". The parser tunes to a particular channel
	// so that whitespace etc... can go to the parser on a "hidden" channel.
	TokenDefaultChannel = 0

	// TokenHiddenChannel defines the normal hidden channel - the parser wil not see tokens that are not on [TokenDefaultChannel].
	//
	// Anything on a different channel than TokenDefaultChannel is not parsed by parser.
	TokenHiddenChannel = 1
)

func (b *BaseToken) GetChannel() int {
	return b.channel
}

func (b *BaseToken) GetStart() int {
	return b.start
}

func (b *BaseToken) GetStop() int {
	return b.stop
}

func (b *BaseToken) GetLine() int {
	return b.line
}

func (b *BaseToken) GetColumn() int {
	return b.column
}

func (b *BaseToken) GetTokenType() int {
	return b.tokenType
}

func (b *BaseToken) GetSource() *TokenSourceCharStreamPair {
	return b.source
}

func (b *BaseToken) GetText() string {
	if b.text != "" {
		return b.text
	}
	input := b.GetInputStream()
	if input == nil {
		return ""
	}
	n := input.Size()
	if b.GetStart() < n && b.GetStop() < n {
		return input.GetTextFromInterval(NewInterval(b.GetStart(), b.GetStop()))
	}
	return "<EOF>"
}

func (b *BaseToken) SetText(text string) {
	b.text = text
}

func (b *BaseToken) GetTokenIndex() int {
	return b.tokenIndex
}

func (b *BaseToken) SetTokenIndex(v int) {
	b.tokenIndex = v
}

func (b *BaseToken) GetTokenSource() TokenSource {
	return b.source.tokenSource
}

func (b *BaseToken) GetInputStream() CharStream {
	return b.source.charStream
}

func (b *BaseToken) String() string {
	txt := b.GetText()
	if txt != "" {
		txt = strings.Replace(txt, "\n", "\\n", -1)
		txt = strings.Replace(txt, "\r", "\\r", -1)
		txt = strings.Replace(txt, "\t", "\\t", -1)
	} else {
		txt = "<no text>"
	}

	var ch string
	if b.GetChannel() > 0 {
		ch = ",channel=" + strconv.Itoa(b.GetChannel())
	} else {
		ch = ""
	}

	return "[@" + strconv.Itoa(b.GetTokenIndex()) + "," + strconv.Itoa(b.GetStart()) + ":" + strconv.Itoa(b.GetStop()) + "='" +
		txt + "',<" + strconv.Itoa(b.GetTokenType()) + ">" +
		ch + "," + strconv.Itoa(b.GetLine()) + ":" + strconv.Itoa(b.GetColumn()) + "]"
}

type CommonToken struct {
	BaseToken
}

var commonTokenPool = sync.Pool{
	New: func() interface{} {
		return new(CommonToken)
	},
}

func (t *CommonToken) Reset() {
	t.source = nil
	t.tokenType = TokenInvalidType
	t.channel = TokenDefaultChannel
	t.start = -1
	t.stop = -1
	t.tokenIndex = -1
	t.line = -1
	t.column = -1
	t.text = ""
	t.readOnly = false
}

func NewCommonToken(source *TokenSourceCharStreamPair, tokenType, channel, start, stop int) *CommonToken {
	t := commonTokenPool.Get().(*CommonToken)
	t.Reset() // Ensure a clean state

	t.source = source
	t.tokenType = tokenType
	t.channel = channel
	t.start = start
	t.stop = stop
	// t.tokenIndex is already -1 from Reset()

	if source != nil && source.tokenSource != nil {
		t.line = source.tokenSource.GetLine()
		t.column = source.tokenSource.GetCharPositionInLine()
	} else {
		// t.line is -1 from Reset()
		t.column = -1 // Explicitly set column if source or tokenSource is nil
	}
	return t
}

// An empty {@link Pair} which is used as the default value of
// {@link //source} for tokens that do not have a source.

//CommonToken.EMPTY_SOURCE = [ nil, nil ]

// Constructs a New{@link CommonToken} as a copy of another {@link Token}.
//
// <p>
// If {@code oldToken} is also a {@link CommonToken} instance, the newly
// constructed token will share a reference to the {@link //text} field and
// the {@link Pair} stored in {@link //source}. Otherwise, {@link //text} will
// be assigned the result of calling {@link //GetText}, and {@link //source}
// will be constructed from the result of {@link Token//GetTokenSource} and
// {@link Token//GetInputStream}.</p>
//
// @param oldToken The token to copy.
func (c *CommonToken) clone() *CommonToken {
	// Instead of NewCommonToken to avoid double reset and specific field setting logic,
	// get from pool and manually copy.
	t := commonTokenPool.Get().(*CommonToken)
	t.Reset() // Ensure clean state before copying

	t.source = c.source // This is a pointer, so it's shared. This is existing behavior.
	t.tokenType = c.tokenType
	t.channel = c.channel
	t.start = c.start
	t.stop = c.stop
	t.tokenIndex = c.tokenIndex // Use GetTokenIndex() if there's logic there
	t.line = c.line             // Use GetLine() if there's logic there
	t.column = c.column         // Use GetColumn() if there's logic there
	t.text = c.text             // Use GetText() if there's logic there (e.g., lazy loading)
	// Note: GetText() on BaseToken can compute text from input stream if t.text is empty.
	// If c.text was empty, c.GetText() would have fetched it.
	// We directly copy c.text to preserve if it was explicitly set or already fetched.
	// If c.text is empty and we want the cloned token to also fetch lazily, this is fine.
	// If c.text was fetched by c.GetText() and we want the clone to have it pre-filled, this is also fine.
	t.readOnly = c.readOnly

	return t
}
