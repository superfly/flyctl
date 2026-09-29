//go:build js && wasm

package main

import (
	"errors"
	"syscall/js"
)

// promise runs fn on its own goroutine and returns a Promise for its
// result. Every exported call goes through it: a Go callback invoked from
// JavaScript must not block, and everything here talks to the network.
func promise(fn func() (any, error)) js.Value {
	var handler js.Func
	handler = js.FuncOf(func(_ js.Value, args []js.Value) any {
		resolve, reject := args[0], args[1]

		go func() {
			defer handler.Release()

			v, err := fn()
			if err != nil {
				reject.Invoke(jsError(err))

				return
			}

			resolve.Invoke(js.ValueOf(v))
		}()

		return nil
	})

	return js.Global().Get("Promise").New(handler)
}

func jsError(err error) js.Value {
	return js.Global().Get("Error").New(err.Error())
}

// await blocks the calling goroutine until v settles, if it's a thenable;
// any other value is returned as is.
func await(v js.Value) (js.Value, error) {
	if v.Type() != js.TypeObject || v.Get("then").Type() != js.TypeFunction {
		return v, nil
	}

	done := make(chan struct{})
	var (
		result js.Value
		err    error
	)

	onOK := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) > 0 {
			result = args[0]
		}
		close(done)

		return nil
	})
	defer onOK.Release()

	onErr := js.FuncOf(func(_ js.Value, args []js.Value) any {
		msg := "promise rejected"
		if len(args) > 0 {
			msg = args[0].String()
			if m := args[0].Get("message"); m.Type() == js.TypeString {
				msg = m.String()
			}
		}
		err = errors.New(msg)
		close(done)

		return nil
	})
	defer onErr.Release()

	v.Call("then", onOK, onErr)
	<-done

	return result, err
}

func bytesToJS(b []byte) js.Value {
	arr := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(arr, b)

	return arr
}

// bytesFromJS accepts a Uint8Array, an ArrayBuffer or a string.
func bytesFromJS(v js.Value) ([]byte, error) {
	switch {
	case v.Type() == js.TypeString:
		return []byte(v.String()), nil
	case v.InstanceOf(js.Global().Get("ArrayBuffer")):
		v = js.Global().Get("Uint8Array").New(v)
		fallthrough
	case v.InstanceOf(js.Global().Get("Uint8Array")):
		b := make([]byte, v.Get("byteLength").Int())
		js.CopyBytesToGo(b, v)

		return b, nil
	default:
		return nil, errors.New("expected a Uint8Array, ArrayBuffer or string")
	}
}

func optString(opts js.Value, key, def string) string {
	v := opts.Get(key)
	if v.Type() != js.TypeString || v.String() == "" {
		return def
	}

	return v.String()
}

func optInt(opts js.Value, key string, def int) int {
	v := opts.Get(key)
	if v.Type() != js.TypeNumber {
		return def
	}

	return v.Int()
}

func optBool(opts js.Value, key string) bool {
	v := opts.Get(key)

	return v.Type() == js.TypeBoolean && v.Bool()
}

func optFunc(opts js.Value, key string) js.Value {
	v := opts.Get(key)
	if v.Type() != js.TypeFunction {
		return js.Undefined()
	}

	return v
}
