package secrets

// This is common implementation between internal packages and flyctl.
// Sync between nmf:kms/kmsfs/keys_common.go and flyctl:internal/command/secrets/keys_common.go

import (
	"fmt"
	"regexp"
	"strconv"
)

type SemanticType string

const (
	SemTypeSigning    = SemanticType("signing")
	SemTypeEncrypting = SemanticType("encrypting")
)

type KeyTypeInfo struct {
	secretType   SecretType
	semanticType SemanticType
}

// supportedKeyTypes lists all supported key types with their semantic key type.
// In this list, the most preferred types are listed first.
var supportedKeyTypes = []KeyTypeInfo{
	// Preferred key types:
	{SECRET_TYPE_KMS_NACL_AUTH, SemTypeSigning},
	{SECRET_TYPE_KMS_NACL_SECRETBOX, SemTypeEncrypting},

	// Also supported key types:
	{SECRET_TYPE_KMS_HS256, SemTypeSigning},
	{SECRET_TYPE_KMS_HS384, SemTypeSigning},
	{SECRET_TYPE_KMS_HS512, SemTypeSigning},
	{SECRET_TYPE_KMS_XAES256GCM, SemTypeEncrypting},

	// Unsupported:
	// SECRET_TYPE_KMS_NACL_BOX, SemTypePublicEncrypting
	// SECRET_TYPE_KMS_NACL_SIGN, SmeTypePublicSigning
}

// SupportedSecretTypes is a list of the SecretTypes for supported key types.
var SupportedSecretTypes = GetSupportedSecretTypes()

// SupportedSecretTypes is a list of the SemanticTypes for supported key types.
var SupportedSemanticTypes = GetSupportedSemanticTypes()

func GetSupportedSecretTypes() []SecretType {
	var r []SecretType
	seen := map[SecretType]bool{}
	for _, info := range supportedKeyTypes {
		st := info.secretType
		if !seen[st] {
			seen[st] = true
			r = append(r, st)
		}
	}
	return r
}

func GetSupportedSemanticTypes() []SemanticType {
	var r []SemanticType
	seen := map[SemanticType]bool{}
	for _, info := range supportedKeyTypes {
		st := info.semanticType
		if !seen[st] {
			seen[st] = true
			r = append(r, st)
		}
	}
	return r
}

func SecretTypeToSemanticType(st SecretType) (SemanticType, error) {
	for _, info := range supportedKeyTypes {
		if info.secretType == st {
			return info.semanticType, nil
		}
	}
	var r SemanticType
	return r, fmt.Errorf("unsupported secret type %s", st)
}

func SemanticTypeToSecretType(st SemanticType) (SecretType, error) {
	for _, info := range supportedKeyTypes {
		if info.semanticType == st {
			return info.secretType, nil
		}
	}

	var r SecretType
	return r, fmt.Errorf("unsupported semantic type %s. use one of %v", st, SupportedSemanticTypes)
}

// Keyver is a key version.
type Keyver int64

const (
	KeyverUnspec Keyver = -1
	KeyverZero   Keyver = 0
	KeyverMax    Keyver = 0x7fff_ffff_ffff_ffff // 9223372036854775807, 19 digits.
)

func (v Keyver) String() string {
	if v == KeyverUnspec {
		return "unspec"
	}
	return fmt.Sprintf("%d", int64(v))
}

func (v Keyver) Incr() (Keyver, error) {
	if v >= KeyverMax {
		return KeyverUnspec, fmt.Errorf("cannot increment version beyond maximum")
	}
	return v + 1, nil
}

func CompareKeyver(a, b Keyver) int {
	d := int64(a) - int64(b)
	switch {
	case d < 0:
		return -1
	case d == 0:
		return 0
	case d > 0:
		return 1
	default:
		return 0
	}
}

// labelPat is a regexp that determines which labels are valid.
// Importantly labels should not have Nil, slashes (we use them in kmsfs paths), colons
// (we use colons as a separator in signatures and ciphertexts that include labels as tags),
// or commas (we use commas to separate multiple arguments, which could include labels).
var labelPat = regexp.MustCompile("^[a-zA-Z0-9_-]+$")

// validKeyLabel determines if a key label is valid or not.
func ValidKeyLabel(label string) error {
	m := labelPat.FindStringSubmatch(label)
	if m == nil {
		return fmt.Errorf("invalid label")
	}
	return nil
}

var labelVersionPat = regexp.MustCompile("^(.*)v([0-9]{1,19})$")

// splitLabelKeyver splits a label into an integer version and the remaining label.
// It returns a version of KeyverUnspec if no label is present or if it would be out of range.
func SplitLabelKeyver(label string) (Keyver, string, error) {
	if err := ValidKeyLabel(label); err != nil {
		return KeyverUnspec, "", err
	}

	m := labelVersionPat.FindStringSubmatch(label)
	if m == nil {
		return KeyverUnspec, label, nil
	}

	l, nstr := m[1], m[2]
	n, _ := strconv.ParseUint(nstr, 10, 64)
	ver := Keyver(n)
	if !(KeyverZero <= ver && ver <= KeyverMax) {
		return KeyverUnspec, label, nil
	}

	return ver, l, nil
}

// JoinLabelVersion adds a keyversion to a key label.
func JoinLabelVersion(ver Keyver, prefix string) string {
	if ver == KeyverUnspec {
		return prefix
	}
	return fmt.Sprintf("%sv%d", prefix, int64(ver))
}
