package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

// rodPage owns page-scoped browser operations. Network admission remains in
// RodDriver so navigation and subresources share one security boundary.
type rodPage struct {
	page          *rod.Page
	router        *rod.HijackRouter
	routerDone    <-chan struct{}
	removeHeaders func()
	closeOnce     sync.Once
	closeErr      error
}

func (page *rodPage) Navigate(ctx context.Context, target string) error {
	current := page.page.Context(nonNilContext(ctx))
	wait := current.WaitNavigation(proto.PageLifecycleEventNameDOMContentLoaded)
	if err := current.Navigate(target); err != nil {
		return err
	}
	wait()
	if err := contextError(ctx); err != nil {
		return err
	}
	return page.guardCurrentURL(ctx)
}

func (page *rodPage) Observe(ctx context.Context) (Observation, error) {
	if err := page.guardCurrentURL(ctx); err != nil {
		return Observation{}, err
	}
	result, err := page.page.Context(nonNilContext(ctx)).Eval(rodObservationScript, maxObservedElements, maxObservationTextRunes)
	if err != nil {
		return Observation{}, err
	}
	raw, err := result.Value.MarshalJSON()
	if err != nil {
		return Observation{}, fmt.Errorf("encode browser observation: %w", err)
	}
	var observation Observation
	if err := json.Unmarshal(raw, &observation); err != nil {
		return Observation{}, fmt.Errorf("decode browser observation: %w", err)
	}
	return observation, nil
}

func (page *rodPage) Wait(ctx context.Context, condition WaitCondition) error {
	if err := page.guardCurrentURL(ctx); err != nil {
		return err
	}
	current := page.page.Context(nonNilContext(ctx))
	err := current.Wait(rod.Eval(`(selector, text) => {
		if (selector) {
			const element = document.querySelector(selector);
			if (!element) return false;
			const style = getComputedStyle(element);
			const rect = element.getBoundingClientRect();
			if (style.visibility === 'hidden' || style.display === 'none' || rect.width <= 0 || rect.height <= 0) return false;
		}
		if (text && !String(document.body && document.body.innerText || '').includes(text)) return false;
		return true;
	}`, condition.Selector, condition.Text))
	if err != nil {
		return err
	}
	return page.guardCurrentURL(ctx)
}

func (page *rodPage) Click(ctx context.Context, selector string) error {
	if err := page.guardCurrentURL(ctx); err != nil {
		return err
	}
	element, err := page.page.Context(nonNilContext(ctx)).Element(selector)
	if err != nil {
		return err
	}
	return element.Click(proto.InputMouseButtonLeft, 1)
}

func (page *rodPage) Fill(ctx context.Context, selector, text string) error {
	if err := page.guardCurrentURL(ctx); err != nil {
		return err
	}
	element, err := page.page.Context(nonNilContext(ctx)).Element(selector)
	if err != nil {
		return err
	}
	if err := element.SelectAllText(); err != nil {
		return err
	}
	return element.Input(text)
}

func (page *rodPage) Type(ctx context.Context, selector, text string) error {
	if err := page.guardCurrentURL(ctx); err != nil {
		return err
	}
	element, err := page.page.Context(nonNilContext(ctx)).Element(selector)
	if err != nil {
		return err
	}
	return element.Input(text)
}

func (page *rodPage) Press(ctx context.Context, selector, keyName string) error {
	if err := page.guardCurrentURL(ctx); err != nil {
		return err
	}
	current := page.page.Context(nonNilContext(ctx))
	if strings.TrimSpace(selector) != "" {
		element, err := current.Element(selector)
		if err != nil {
			return err
		}
		if err := element.Focus(); err != nil {
			return err
		}
	}
	key, err := browserKey(keyName)
	if err != nil {
		return err
	}
	return current.Keyboard.Type(key)
}

func (page *rodPage) Select(ctx context.Context, selector string, values []string) error {
	if err := page.guardCurrentURL(ctx); err != nil {
		return err
	}
	element, err := page.page.Context(nonNilContext(ctx)).Element(selector)
	if err != nil {
		return err
	}
	result, err := element.Eval(`(values) => {
		const wanted = new Set(values.map(String));
		let matched = 0;
		for (const option of this.options || []) {
			option.selected = wanted.has(String(option.value)) || wanted.has(String(option.textContent || '').trim());
			if (option.selected) matched++;
		}
		this.dispatchEvent(new Event('input', { bubbles: true }));
		this.dispatchEvent(new Event('change', { bubbles: true }));
		return matched === wanted.size;
	}`, values)
	if err != nil {
		return err
	}
	if !result.Value.Bool() {
		return errors.New("one or more browser select values did not match an option")
	}
	return nil
}

func (page *rodPage) Evaluate(ctx context.Context, expression string) (json.RawMessage, error) {
	if err := page.guardCurrentURL(ctx); err != nil {
		return nil, err
	}
	result, err := page.page.Context(nonNilContext(ctx)).Evaluate(
		rod.Eval("async () => await (" + expression + ")").ByPromise(),
	)
	if err != nil {
		return nil, err
	}
	raw, err := result.Value.MarshalJSON()
	if err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

func (page *rodPage) Screenshot(ctx context.Context, fullPage bool) ([]byte, error) {
	if err := page.guardCurrentURL(ctx); err != nil {
		return nil, err
	}
	return page.page.Context(nonNilContext(ctx)).Screenshot(fullPage, nil)
}

func (page *rodPage) guardCurrentURL(ctx context.Context) error {
	if page == nil || page.page == nil {
		return errors.New("browser page is not configured")
	}
	info, err := page.page.Context(nonNilContext(ctx)).Info()
	if err != nil {
		return fmt.Errorf("read browser page URL: %w", err)
	}
	current := strings.TrimSpace(info.URL)
	if current == "about:blank" {
		return nil
	}
	if _, err := ValidatePublicURL(ctx, current); err != nil {
		return fmt.Errorf("browser page left the public HTTP(S) boundary: %w", err)
	}
	return nil
}

func (page *rodPage) Close(context.Context) error {
	if page == nil {
		return nil
	}
	page.closeOnce.Do(func() {
		var closeErrors []error
		if page.removeHeaders != nil {
			page.removeHeaders()
		}
		if page.router != nil {
			if err := page.router.Stop(); err != nil {
				closeErrors = append(closeErrors, err)
			}
		}
		if page.routerDone != nil {
			<-page.routerDone
		}
		if page.page != nil {
			if err := page.page.Close(); err != nil {
				closeErrors = append(closeErrors, err)
			}
		}
		page.closeErr = errors.Join(closeErrors...)
	})
	return page.closeErr
}

const rodObservationScript = `(maxElements, maxText) => {
	const clean = (value) => String(value || '').replace(/\s+/g, ' ').trim();
	const roleFor = (el) => el.getAttribute('role') || ({
		A: 'link', BUTTON: 'button', INPUT: el.type === 'checkbox' ? 'checkbox' : 'textbox',
		TEXTAREA: 'textbox', SELECT: 'combobox', H1: 'heading', H2: 'heading', H3: 'heading'
	}[el.tagName] || el.tagName.toLowerCase());
	const visible = (el) => {
		const style = getComputedStyle(el);
		const rect = el.getBoundingClientRect();
		return style.visibility !== 'hidden' && style.display !== 'none' && rect.width > 0 && rect.height > 0;
	};
	const candidates = Array.from(document.querySelectorAll(
		'a,button,input,textarea,select,[role],[contenteditable="true"],h1,h2,h3'
	)).filter(visible);
	const elements = candidates.slice(0, maxElements).map((el, index) => {
		const ref = 'e' + (index + 1);
		el.setAttribute('data-denova-ref', ref);
		const name = clean(el.getAttribute('aria-label') || el.getAttribute('title') ||
			el.getAttribute('alt') || el.getAttribute('placeholder') || el.innerText || el.textContent);
		return { ref, role: roleFor(el), name, selector: '[data-denova-ref="' + ref + '"]' };
	});
	const text = clean(document.body && document.body.innerText);
	return {
		url: location.href, title: document.title || '', text: text.slice(0, maxText), elements,
		truncated: text.length > maxText || candidates.length > maxElements
	};
}`

func browserKey(name string) (input.Key, error) {
	name = strings.TrimSpace(name)
	keys := map[string]input.Key{
		"Enter": input.Enter, "Escape": input.Escape, "Tab": input.Tab,
		"Backspace": input.Backspace, "Delete": input.Delete, "Space": input.Space,
		"ArrowLeft": input.ArrowLeft, "ArrowRight": input.ArrowRight,
		"ArrowUp": input.ArrowUp, "ArrowDown": input.ArrowDown,
		"PageUp": input.PageUp, "PageDown": input.PageDown, "Home": input.Home, "End": input.End,
	}
	if key, ok := keys[name]; ok {
		return key, nil
	}
	runeValue, size := utf8.DecodeRuneInString(name)
	if size == len(name) && runeValue >= 0x20 && runeValue <= 0x7e {
		return input.Key(runeValue), nil
	}
	return 0, fmt.Errorf("unsupported browser key %q", name)
}
