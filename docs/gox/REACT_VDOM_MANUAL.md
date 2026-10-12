# GoX React and VDOM Manual

GoX provides a small React-like component runtime for Go. Components are
ordinary Go code written with GoX syntax. The runtime owns component identity,
state, context, effects, and reconciliation. A renderer is a separate host
adapter: it receives virtual nodes and translates them into a window, terminal,
browser, GPU surface, or another output system.

The runtime is experimental. It does not include a built-in GUI renderer.

## 1. Add the runtime to an application

Import the runtime package in ordinary Go files that create the root or host
adapter:

```go
import view "github.com/alfu32/go-shared/gox/goxview"
```

GoX component source can refer to this runtime as `vdom` without declaring an
import. For example, a component may accept `children []vdom.Node`; the GoX
compiler supplies the runtime import. The application still needs the GoX
compiler to process `.gox` source. See [the GoX README](../../gox/README.md) for
building and running GoX programs.

## 2. Write a component

A component declares typed, required properties and returns one VDOM node.
Lowercase markup tags create host elements; capitalized tags refer to
components. Markup children can include text, nodes, node slices, and `nil`.
Use an `F` string to interpolate Go values into text.

```go
var Theme = view.NewContext("light")

component Counter(title string) {
    count, setCount := useState(0)
    theme := useContext(Theme)

    return X`<button theme={theme} onClick={func() {
        setCount.Update(func(latest int) int { return latest + 1 })
    }}>{F"{title}: {count}"}</button>`
}
```

The equivalent bare markup form is also accepted:

```go
return <button theme={theme}>{F"{title}: {count}"}</button>
```

`X"..."` and bare markup lower to the same kind of nodes. Attribute values in
braces remain typed Go values, so an event handler stays a function instead of
being converted to text. A `key` attribute sets occurrence identity metadata;
it is not passed to the host as an ordinary attribute.

Component properties are passed by name and are mandatory unless their Go type
allows a meaningful zero value and the property is still supplied. Missing or
unknown properties fail GoX's generated type checks. An optional
`children []vdom.Node` component parameter receives nested markup children.

## 3. Hooks

Hooks are called inside a component, directly and unconditionally at the start
of its body, before branches, loops, or other control flow. Their order and
types must remain stable across renders. GoX checks this placement when it
transforms a component.

### State

`useState(initial)` returns the current value and a setter. Calling the setter
replaces the value; `Update` applies a function to the latest queued value:

```go
count, setCount := useState(0)

// In a host callback:
setCount(count + 1)
setCount.Update(func(latest int) int { return latest + 1 })
```

State changes are queued. A root flush or runner frame processes queued changes
and renders again if any live component changed. Setters from worker goroutines
should use `Enqueue(value)` or `EnqueueUpdate(func)`; these methods are safe to
queue while rendering. The supplied `wake` function should signal the host's
event loop, rather than rendering synchronously.

### Refs

`useRef(initial)` returns a persistent pointer with a `Current` field:

```go
renderCount := useRef(0)
renderCount.Current++
```

Changing a ref does not request a render. Use state when changing a value must
cause the UI to be reconciled.

### Context

Create a typed context once, outside component rendering, and wrap the part of
the tree that should receive a value:

```go
var Theme = view.NewContext("light")

tree := Theme.Provider("dark",
    <Counter key="primary" title="Primary"/>,
)
```

Inside a descendant component, `useContext(Theme)` returns the nearest provider
value or the context's fallback. Context values are read during rendering.

### Effects

`useEffect` runs after a successful render has committed. Return a cleanup
function to release resources before the effect runs again or the component
unmounts:

```go
useEffect(func() func() {
    cancel := subscribe(topic)
    return func() { cancel() }
}, topic)
```

With dependencies, the effect reruns when a dependency changes (compared using
deep equality). With no dependencies, it runs after each successful render.
Cleanup also runs when the component is removed or the root is unmounted.

## 4. Mount and reconcile a tree

The headless root is useful for simple applications and tests:

```go
root := view.NewRoot(commit, wake)
if err := root.Mount(tree); err != nil {
    return err
}

// After an event handler queues state changes:
if err := root.Flush(); err != nil {
    return err
}

currentTree := root.Tree()
root.Unmount()
```

`commit` may be `nil`; otherwise it receives each successfully committed tree.
`wake` may also be `nil`; otherwise it signals that queued state work is ready.
`Tree()` returns the current resolved host tree. `Unmount()` runs effect cleanups,
invalidates setters belonging to removed component instances, clears pending
updates, and commits an empty tree.

Ordinary Go can construct a host tree directly when a component system is not
needed:

```go
tree := view.Element("panel",
    []view.Attr{{Name: "title", Value: "Status"}},
    []view.Node{view.Text("Ready")},
)
root := view.NewRoot(nil, nil)
if err := root.Mount(tree); err != nil {
    return err
}
resolved := root.Tree()
```

`resolved` is a host node with a text child. With a renderer, pass its
`Render` method as the root's commit callback; the renderer decides what
`panel` and `title` mean on its host platform.

Component instance identity follows its parent path, component definition, and
either its explicit key or sibling position. Keys may be strings or integers
and must be unique among siblings. Keeping a key stable preserves that
component's state when siblings move. Changing the key or component definition
creates a fresh instance. Without explicit keys, sibling position is used.

The reconciler currently resolves the whole root on each render; it does not do
selective subtree updates. A render panic is returned as an error and leaves
the prior committed tree in place. Side effects performed by user code during
render, including ref changes, cannot be rolled back.

## 5. Implement a renderer

The renderer owns drawing and platform details. It receives a resolved `Node`
tree after each commit:

```go
type CanvasRenderer struct { /* host-specific state */ }

func (c *CanvasRenderer) Render(root view.Node) {
    // Walk root, draw text and elements, and retain any host-side state needed
    // for event routing. Node has Tag, Text, Attrs, Children, and Key fields.
}
```

Implement `view.Renderer` by providing `Render(view.Node)`. The runtime does not
interpret host tags, draw nodes, or prescribe an attribute schema. A renderer
decides which tags and attributes it supports and how function-valued
attributes map to host callbacks.

The public VDOM building blocks are:

| API | Purpose |
| --- | --- |
| `view.Node` | Host element, text node, or a node awaiting component resolution |
| `view.Attr` | Attribute name and typed value |
| `view.Element(tag, attrs, children)` | Build a host element in ordinary Go |
| `view.Text(text)` | Build a text node |
| `view.Child(value)` | Convert `nil`, a node, node slice, or string to children |
| `view.Define(id, render)` | Define a component for the low-level Go API |
| `view.Component(definition, attrs, children)` | Build a low-level component occurrence |

Most applications should use GoX component declarations and markup rather than
constructing component definitions by hand.

## 6. Add input and a per-frame clock

For an interactive application, use the runner to connect a renderer, an
application event model, and the reconciler:

```go
type App struct{}

func (a *App) Update(event view.Event) {
    switch e := event.(type) {
    case view.ClockEvent:
        // Update animation or time-based model state.
    case view.KeyboardEvent:
        // Respond to keyboard input.
    case view.MouseEvent:
        // Respond to pointer input.
    }
}

runner := view.NewRunner(renderer, &App{}, wake)
if err := runner.Root.Mount(tree); err != nil {
    return err
}

// Called by the host once for each rendering frame:
if err := runner.Frame(time.Now()); err != nil {
    return err
}
```

The model implements `Update(view.Event)`. Every `Frame(now)` runs scheduled
work, delivers queued input events in order, sends a `ClockEvent`, and flushes
state updates. `ClockEvent` contains the frame time, elapsed time since the
previous frame, and a one-based frame number. The first frame has a zero delta.

A renderer that also implements `view.InteractiveRenderer` provides
`SetEventSink(func(view.Event))`. It calls that sink to enqueue input. The
runtime defines `KeyboardEvent` and `MouseEvent` data types, but the renderer
must translate its platform's input into those values. Other event values can
also be passed to `runner.Dispatch`.

Use `runner.Schedule(func())` to queue work from other goroutines. Scheduled
work runs on the host goroutine at the next frame. State setters used by worker
goroutines should use their `Enqueue` variants. All rendering and root
operations (`Mount`, `Flush`, `Tree`, and `Unmount`) belong to one host
goroutine.

## 7. Limitations

- The runtime has no built-in browser, desktop, terminal, or GPU renderer.
- Host element and attribute meaning is entirely renderer-defined.
- Reconciliation currently traverses the full tree on each render.
- Hooks must remain at the beginning of components and cannot be conditional.
- The runtime ABI and generated GoX component adapters are experimental.

The runnable counter in [`gox/examples/counter`](../../gox/examples/counter)
shows GoX components, state, refs, context, markup callbacks, and a headless
root in one small program.
