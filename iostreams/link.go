package iostreams

import (
	"fmt"
)

func CreateLink(text string, url string) string {
	fmt.Println("jhello")
	// if canMakeTextHyperlink() {
	// 	return "\x1b]8;;" + url + "\x07" + text + "\x1b]8;;\x07"
	// } else {
	// 	return text + " (\u200B" + url + ")"
	// }
	return "\x1b]8;;" + url + "\x07" + text + "\x1b]8;;\x07"
}
