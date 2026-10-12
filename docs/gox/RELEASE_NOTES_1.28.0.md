# GoX 1.28.0 Release Notes

This release adds F strings, X strings for virtual DOM (VDOM) literals, and a
React-like component runtime with a reconciler. The runtime is renderer-neutral:
it builds and reconciles VDOM trees, while a host renderer decides how to draw
them and how to turn keyboard and mouse input into application events.

These features are experimental. There is no built-in GUI renderer.

## F strings

Prefix a quoted or raw string with uppercase `F` to interpolate Go expressions.
An unformatted hole uses `%v`; add a Go formatting verb after a colon to choose
another format. Double braces to write literal braces. For example:

```go
name := "Ada"
count := 7
message := F"Hello, {name}: {count:%03d}"
raw := F`Use {{ and }} for literal braces; name={name}`
```

The first value is equivalent to `fmt.Sprintf("Hello, %v: %03d", name, count)`.
An interpolated string is lowered to one `fmt.Sprintf` call. Each hole is
evaluated once, in source order. Literal percent signs are escaped in the
generated format string. An F string with no holes remains a Go constant.

F strings can be nested and can appear inside markup:

```go
label := F"total: {F"{count} items"}"
```

## X strings and VDOM literals

An `X` string contains markup that lowers to VDOM construction. Raw string
quotes are useful when markup contains ordinary quotes. Bare markup is also
accepted in expression position and lowers to the same VDOM values.

```go
title := "Overview"
node := X`<section class="summary">
  <h1>{title}</h1>
  <p>{F"Items: {count:%d}"}</p>
</section>`

sameNode := <section class="summary"><h1>{title}</h1></section>
```

Lowercase tags create host elements. Capitalized or qualified tags refer to
components. Attributes accept literal text (`class="summary"`) or typed Go
expressions (`onClick={handleClick}`); expression values remain Go values and
are not stringified. Markup children can be strings, VDOM nodes, node slices,
or `nil`. Use an F string when a value should be formatted as text. Markup text
and whitespace are preserved, and each literal has one root element. The `key`
attribute supplies component occurrence identity and is not passed to the host
as a normal attribute.

In GoX source, `vdom` is the implicit name for the runtime package, so a
component can use `[]vdom.Node` without importing it. Ordinary Go files can
import the runtime explicitly:

```go
import view "github.com/alfu32/go-shared/gox/goxview"

tree := view.Element("p", nil, []view.Node{view.Text("Ready")})
```

The core VDOM API includes `Node`, `Attr`, `Element`, `Text`, and `Child` for
host trees, plus `Component` and `Define` for the low-level component API.

## Components and reconciliation

Declare a component with typed properties and return one node. GoX generates
type checks for component properties, including checks for missing or unknown
properties. Nested markup is passed through an optional `children []vdom.Node`
property.

```go
component Counter(title string) {
    count, setCount := useState(0)
    return <button onClick={func() {
        setCount.Update(func(latest int) int { return latest + 1 })
    }}>{F"{title}: {count:%d}"}</button>
}

page := <Counter key="main" title="Clicks"/>
```

The reconciler tracks mounted component instances and hook data across renders.
Identity follows the parent path, component definition, and explicit `key` (or
sibling position if no key is provided). Stable keys preserve state when keyed
siblings move; changing the key creates a fresh instance. Keys must be unique
among siblings. A render currently reconciles the whole root tree.

### Built-in hooks

Hooks must be called directly and in a stable order at the beginning of a
component body, before branches and loops.

- `useState(initial)` stores state and returns a setter. Setters replace state;
  `Update` derives a value from the latest queued state. Worker goroutines can
  use `Enqueue` and `EnqueueUpdate` to queue changes safely.
- `useRef(initial)` returns a persistent `Current` value. Changing it does not
  schedule a render.
- `useContext(context)` reads the closest provider value, or the context's
  default. Create contexts with `vdom.NewContext` and scope values with
  `Provider(value, children...)`.
- `useEffect(func() cleanup, dependencies...)` runs after a successful commit.
  Cleanup runs before a changed effect is replaced and when its component is
  removed. With dependencies, the effect reruns when they change; without
  dependencies, it runs after every successful render.

The equivalent exported runtime methods are `UseState`, `UseRef`, `UseContext`,
and `UseEffect`.

### Roots and renderer boundary

Mount a tree in a headless root, then flush after handlers queue state changes:

```go
root := vdom.NewRoot(renderer.Render, wake)
if err := root.Mount(page); err != nil {
    return err
}
// After an input handler changes state:
if err := root.Flush(); err != nil {
    return err
}
root.Unmount()
```

A renderer implements `Render(vdom.Node)` and receives each resolved tree after
a successful commit. It owns tag and attribute interpretation, drawing, and
host-specific event routing. An `InteractiveRenderer` can expose an event sink
for input events. This separation allows renderers for GUI, terminal, browser,
or other hosts without coupling the reconciler to a drawing toolkit.

### Runner, work queue, and frame events

`vdom.NewRunner(renderer, model, wake)` connects the renderer and reconciler to
an application model that implements `Update(vdom.Event)`. It provides
`Schedule(func())` for queued work and `Dispatch(event)` for input. A host calls
`Frame(now)` once per rendering frame:

```go
type App struct{}

func (*App) Update(event vdom.Event) {
    switch e := event.(type) {
    case vdom.ClockEvent:
        animate(e.Delta)
    case vdom.KeyboardEvent:
        handleKey(e)
    case vdom.MouseEvent:
        handleMouse(e)
    }
}

runner := vdom.NewRunner(renderer, &App{}, wake)
if err := runner.Frame(time.Now()); err != nil {
    return err
}
```

Each frame runs scheduled work, delivers queued events in order, dispatches a
`ClockEvent`, then flushes state updates and renders. The clock event includes
the current frame time, elapsed time since the previous frame, and a one-based
frame number; its first delta is zero. Keyboard and mouse event types let host
renderers feed input into the same application model.

## Further documentation

The [React and VDOM manual](REACT_VDOM_MANUAL.md) covers mounting, renderer
adapters, hooks, event flow, keys, and runtime behavior in more detail. The
[GoX README](../../gox/README.md) documents the language syntax and commands.
