package interactive

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// EvalRuleExpression evaluates a small, read-only arithmetic expression against
// story state. It intentionally supports only literals, state paths, arithmetic,
// comparisons, and min/max/clamp so rule formulas cannot execute code.
func EvalRuleExpression(expr string, state map[string]any) (float64, error) {
	parser := newRuleExprParser(expr, state)
	value, err := parser.parseComparison()
	if err != nil {
		return 0, err
	}
	parser.skipSpace()
	if !parser.eof() {
		return 0, fmt.Errorf("表达式包含无法识别的内容: %q", parser.remaining())
	}
	return value, nil
}

func ValidateRuleExpression(expr string) error {
	_, err := EvalRuleExpression(expr, map[string]any{})
	return err
}

type ruleExprParser struct {
	input string
	pos   int
	state map[string]any
}

func newRuleExprParser(input string, state map[string]any) *ruleExprParser {
	return &ruleExprParser{input: strings.TrimSpace(input), state: state}
}

func (p *ruleExprParser) parseComparison() (float64, error) {
	left, err := p.parseAddSub()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpace()
		op := p.readComparisonOp()
		if op == "" {
			return left, nil
		}
		right, err := p.parseAddSub()
		if err != nil {
			return 0, err
		}
		left = boolNumber(compareRuleExpr(left, right, op))
	}
}

func (p *ruleExprParser) parseAddSub() (float64, error) {
	left, err := p.parseMulDiv()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpace()
		switch p.peek() {
		case '+':
			p.pos++
			right, err := p.parseMulDiv()
			if err != nil {
				return 0, err
			}
			left += right
		case '-':
			p.pos++
			right, err := p.parseMulDiv()
			if err != nil {
				return 0, err
			}
			left -= right
		default:
			return left, nil
		}
	}
}

func (p *ruleExprParser) parseMulDiv() (float64, error) {
	left, err := p.parseUnary()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpace()
		switch p.peek() {
		case '*':
			p.pos++
			right, err := p.parseUnary()
			if err != nil {
				return 0, err
			}
			left *= right
		case '/':
			p.pos++
			right, err := p.parseUnary()
			if err != nil {
				return 0, err
			}
			if right == 0 {
				return 0, fmt.Errorf("表达式除零")
			}
			left /= right
		default:
			return left, nil
		}
	}
}

func (p *ruleExprParser) parseUnary() (float64, error) {
	p.skipSpace()
	switch p.peek() {
	case '+':
		p.pos++
		return p.parseUnary()
	case '-':
		p.pos++
		value, err := p.parseUnary()
		return -value, err
	default:
		return p.parsePrimary()
	}
}

func (p *ruleExprParser) parsePrimary() (float64, error) {
	p.skipSpace()
	if p.eof() {
		return 0, fmt.Errorf("表达式意外结束")
	}
	if p.peek() == '(' {
		p.pos++
		value, err := p.parseComparison()
		if err != nil {
			return 0, err
		}
		p.skipSpace()
		if p.peek() != ')' {
			return 0, fmt.Errorf("表达式缺少右括号")
		}
		p.pos++
		return value, nil
	}
	if isRuleExprNumberStart(p.peek()) {
		return p.readNumber()
	}
	if isRuleExprIdentStart(p.peek()) {
		ident := p.readIdent()
		p.skipSpace()
		if p.peek() == '(' {
			return p.readFunction(ident)
		}
		return numberFromAny(getPath(p.state, ident)), nil
	}
	return 0, fmt.Errorf("表达式包含不支持的字符: %q", p.peek())
}

func (p *ruleExprParser) readFunction(name string) (float64, error) {
	p.pos++
	args := make([]float64, 0, 3)
	for {
		p.skipSpace()
		if p.peek() == ')' {
			p.pos++
			break
		}
		value, err := p.parseComparison()
		if err != nil {
			return 0, err
		}
		args = append(args, value)
		p.skipSpace()
		switch p.peek() {
		case ',':
			p.pos++
		case ')':
			p.pos++
			goto done
		default:
			return 0, fmt.Errorf("函数参数缺少逗号或右括号")
		}
	}
done:
	switch strings.ToLower(name) {
	case "min":
		if len(args) < 1 {
			return 0, fmt.Errorf("min 至少需要一个参数")
		}
		out := args[0]
		for _, value := range args[1:] {
			out = math.Min(out, value)
		}
		return out, nil
	case "max":
		if len(args) < 1 {
			return 0, fmt.Errorf("max 至少需要一个参数")
		}
		out := args[0]
		for _, value := range args[1:] {
			out = math.Max(out, value)
		}
		return out, nil
	case "clamp":
		if len(args) != 3 {
			return 0, fmt.Errorf("clamp 需要 3 个参数")
		}
		return math.Min(math.Max(args[0], args[1]), args[2]), nil
	default:
		return 0, fmt.Errorf("不支持的函数: %s", name)
	}
}

func (p *ruleExprParser) readComparisonOp() string {
	for _, op := range []string{">=", "<=", "==", "!=", ">", "<"} {
		if strings.HasPrefix(p.input[p.pos:], op) {
			p.pos += len(op)
			return op
		}
	}
	return ""
}

func (p *ruleExprParser) readNumber() (float64, error) {
	start := p.pos
	for !p.eof() {
		ch := p.peek()
		if !unicode.IsDigit(rune(ch)) && ch != '.' {
			break
		}
		p.pos++
	}
	value, err := strconv.ParseFloat(p.input[start:p.pos], 64)
	if err != nil {
		return 0, fmt.Errorf("数字无效: %s", p.input[start:p.pos])
	}
	return value, nil
}

func (p *ruleExprParser) readIdent() string {
	start := p.pos
	for !p.eof() {
		ch := p.peek()
		if !isRuleExprIdentPart(ch) {
			break
		}
		p.pos++
	}
	return p.input[start:p.pos]
}

func (p *ruleExprParser) skipSpace() {
	for !p.eof() && unicode.IsSpace(rune(p.peek())) {
		p.pos++
	}
}

func (p *ruleExprParser) peek() byte {
	if p.eof() {
		return 0
	}
	return p.input[p.pos]
}

func (p *ruleExprParser) eof() bool {
	return p.pos >= len(p.input)
}

func (p *ruleExprParser) remaining() string {
	if p.eof() {
		return ""
	}
	return p.input[p.pos:]
}

func compareRuleExpr(left, right float64, op string) bool {
	switch op {
	case ">":
		return left > right
	case ">=":
		return left >= right
	case "<":
		return left < right
	case "<=":
		return left <= right
	case "==":
		return left == right
	case "!=":
		return left != right
	default:
		return false
	}
}

func boolNumber(value bool) float64 {
	if value {
		return 1
	}
	return 0
}

func isRuleExprNumberStart(ch byte) bool {
	return unicode.IsDigit(rune(ch)) || ch == '.'
}

func isRuleExprIdentStart(ch byte) bool {
	return unicode.IsLetter(rune(ch)) || ch == '_'
}

func isRuleExprIdentPart(ch byte) bool {
	return unicode.IsLetter(rune(ch)) || unicode.IsDigit(rune(ch)) || ch == '_' || ch == '.'
}
