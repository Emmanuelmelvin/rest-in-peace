package cli

import "fmt"

func runCheck(args []string) int {
	_, entry, err := resolve(args)
	if err != nil {
		return fail(err)
	}
	res, ok := compile(entry, false)
	if !ok {
		return 1
	}
	if len(res.Diagnostics) == 0 {
		fmt.Println("NO ERRORS.\nREADY.")
	}
	return 0
}
