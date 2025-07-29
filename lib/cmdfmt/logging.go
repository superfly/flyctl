package cmdfmt

const lineFeed = '\n'

// AppendMissingLineFeed adds \n to a line unless already present
func AppendMissingLineFeed(msg string) string {
	buff := []byte(msg)
	if len(buff) == 0 || buff[len(buff)-1] != lineFeed {
		buff = append(buff, lineFeed)
	}
	return string(buff)
}
