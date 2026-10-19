# Learn Go through Heliolytics

Use this guide for the Go used by this backend. It does not cover the whole language.

## 1. Packages group related files

`package api` at the top of a file says that the file belongs to the API package. Files in that directory share package-level declarations. `metricsHandler` can be declared in `metrics_common.go` and used in `metrics_days.go` without importing that file.

An `import` brings in another package, such as `net/http`. `go.mod` records the module name, Go language version and dependencies. `go.sum` records dependency checksums. It is not equivalent to source code.

## 2. Variables and function calls

Actual line from `internal/api/metrics_days.go`:

```go
rows, err := h.st.ListDays(r.Context(), from, to)
```

`:=` declares variables and infers their types inside a function. `ListDays` returns two values: rows and an error. `=` assigns to an existing variable. `h.st` is the handler's store. `from` and `to` are date strings.

```go
if err != nil {
    return
}
```

`nil` means no value for types that permit it. A nil error means no reported failure. Go usually returns errors instead of throwing exceptions. The real handler writes an HTTP error before returning. This snippet only shows syntax.

## 3. Structs describe data

A simplified example based on `internal/store/metrics_types.go`:

```go
type DayMetric struct {
    DayKey string `json:"dayKey"`
    Steps  int    `json:"steps"`
    Hrv    *int   `json:"hrv,omitempty"`
}
```

This teaching example is not the complete API model. A `struct` groups named fields. The text in backticks tells the JSON encoder which field name to use. An uppercase identifier is exported outside its package. A lowercase identifier is private to the package. Exported struct fields can be handled by the standard JSON encoder.

`*int` is a pointer to an integer. Here it represents an optional metric: `nil` means missing, while a pointer to zero means a present zero. `omitempty` omits a nil pointer from JSON. Without it, a nil pointer is normally encoded as `null`.

## 4. Methods attach behavior to a type

```go
func (h *metricsHandler) days(w http.ResponseWriter, r *http.Request)
```

The `(h *metricsHandler)` part is the receiver. Inside the method, `h` refers to that handler. `*metricsHandler` is a pointer, so the method uses the existing handler instead of copying the struct. Go has no classes or inheritance here.

`*T` in a type means pointer to T. `&value` takes an address. `*pointer` in an expression reads the pointed-to value. Go manages allocated memory. These structs do not require manual deallocation.

## 5. Collections

- `[]DayMetric`: a slice of daily records. Similar to a list, backed by an array.
- `map[string][]byte`: type code to raw byte slice. Used for uploaded blobs.
- `make([]DayMetric, 0, len(rows))`: empty slice with room reserved for results.
- `append(out, item)`: returns a slice with the item added. Assign the result.
- `for _, row := range rows`: visit each row. `_` discards the index.

A map is useful for lookup. Its iteration order is not guaranteed. A slice preserves order. Sorting dates explicitly matters when chart order is part of the result.

## 6. Interfaces describe required behavior

`internal/rollup/target.go` defines the methods rollups need. A type satisfies an interface by providing those methods. It does not write an `implements` declaration. The transaction view lets rollups read and write through the current transaction. This keeps formula code separate from a particular database connection.

## 7. Cleanup, cancellation and concurrency

`defer st.Close()` schedules cleanup when the current function returns. It does not close the database immediately. Deferred calls run in reverse order. `defer` inside a loop still waits for the containing function.

`r.Context()` carries request cancellation to database operations. Passing a context does not start background work. See the [context reference](https://pkg.go.dev/context).

`go func() { ... }()` starts a goroutine. In `cmd/server/main.go`, the HTTP server runs while main waits for a stop signal on a channel. `<-stop` waits to receive. A buffered channel can hold values. On shutdown, main creates a ten-second context and asks HTTP to stop gracefully. Ordinary SQL syntax does not require a goroutine.

## 8. Errors and tools

`fmt.Errorf("parse blobs: %w", err)` adds context while keeping the underlying error available for inspection. `return nil, err` returns no result plus a failure. Check errors before using the result. A panic is not the usual request-error path.

`gofmt` formats code, `go test` runs tests, and `go vet` checks suspicious patterns. `sqlc generate` creates database code from SQL. These commands are described here. The application and its tests were not run for this revision.

Next: [trace an actual request](go-request-walkthrough.md).
For language practice: [A Tour of Go](https://go.dev/tour/).

Validation: not run for this revision.
