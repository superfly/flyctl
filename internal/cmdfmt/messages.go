package cmdfmt

import (
	"context"
	"fmt"

	"github.com/logrusorgru/aurora"
	"github.com/morikuni/aec"

	"github.com/superfly/flyctl/pkg/iostreams"
)

func Println(ctx context.Context, args ...interface{}) {
	out := iostreams.FromContext(ctx).Out

	fmt.Fprintln(out, args...)
}

func Printf(ctx context.Context, format string, args ...interface{}) {
	out := iostreams.FromContext(ctx).Out

	fmt.Fprintf(out, format, args...)
}

func Begin(ctx context.Context, args ...interface{}) {
	Println(ctx, aurora.Green("==> "+fmt.Sprint(args...)))
}

// Done prints to the output ctx carries. It behaves similarly to log.Print.
func Done(ctx context.Context, args ...interface{}) {
	Println(ctx, aurora.Gray(20, "--> "+fmt.Sprint(args...)))
}

// Donef prints to the output ctx carries. It behaves similarly to log.Printf.
func Donef(ctx context.Context, format string, args ...interface{}) {
	Done(ctx, fmt.Sprintf(format, args...))
}

// Detail prints to the output ctx carries. It behaves similarly to log.Print.
func Detail(ctx context.Context, args ...interface{}) {
	Println(ctx, aurora.Faint(fmt.Sprint(args...)))
}

// Detailf prints to the output ctx carries. It behaves similarly to log.Printf.
func Detailf(ctx context.Context, format string, args ...interface{}) {
	Detail(ctx, fmt.Sprintf(format, args...))
}

// StatusLn prints an empty line separator unless JSON output is requested
func Separator(ctx context.Context) {
	// TODO: check for json requirement
	Println(ctx, "")
}

func Overwrite(ctx context.Context) {
	Println(ctx, aec.Up(1))
	Println(ctx, aec.EraseLine(aec.EraseModes.All))
}
