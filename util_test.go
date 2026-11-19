package gogsmmodem

import "fmt"

func Example_parseTime() {
	t := parseTime("14/02/01,15:07:43+00")
	fmt.Println(t)
	// Output:
	// 2014-02-01 15:07:43 +0000 UTC
}

func Example_startsWith() {
	fmt.Println(startsWith("abc", "ab"))
	fmt.Println(startsWith("abc", "b"))
	// Output:
	// true
	// false
}

func Example_quotes() {
	args := []any{"a", 1, "b"}
	fmt.Println(quotes(args))
	// Output:
	// "a",1,"b"
}

func Example_unquotes() {
	fmt.Println(unquotes(`"a,comma",1,"b"`))
	// Output:
	// [a,comma 1 b]
}

func Example_gsmEncode() {
	fmt.Printf("%q\n", gsmEncode("abcdefghijklmnopqrstuvwxyz"))
	fmt.Printf("%q\n", gsmEncode("ABCDEFGHIJKLMNOPQRSTUVWXYZ"))
	fmt.Printf("%q\n", gsmEncode("0123456789"))
	fmt.Printf("%q\n", gsmEncode(".,+-*/ "))
	fmt.Printf("%q\n", gsmEncode("°"))
	fmt.Printf("%q\n", gsmEncode("@£"))
	fmt.Printf("%q\n", gsmEncode("{}"))
	// Output:
	// "abcdefghijklmnopqrstuvwxyz"
	// "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	// "0123456789"
	// ".,+-*/ "
	// ""
	// "\x00\x01"
	// "\x1b(\x1b)"
}
