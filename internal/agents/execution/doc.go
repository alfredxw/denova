// Package execution owns durable Agent command admission, ordering, recovery,
// domain-commit coordination, and settlement. It delegates each admitted model
// cycle to chat.Executor and never owns model-context composition itself.
package execution
