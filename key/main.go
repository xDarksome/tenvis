package key

import (
	"bytes"
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/crypto/ed25519"
)

var (
	base32enc = base32.StdEncoding.WithPadding(base32.NoPadding)
	base64enc = base64.RawStdEncoding
)

type Public struct {
	raw ed25519.PublicKey
	str string
}

func (p *Public) Verify(message, sig []byte) bool {
	return ed25519.Verify(p.raw, message, sig)
}

func (p *Public) String() string {
	return p.str
}

func (p *Public) Ed25519() ed25519.PublicKey {
	return p.raw
}

type Private struct {
	raw ed25519.PrivateKey
	str string
}

func (p *Private) Sign(message []byte) []byte {
	return ed25519.Sign(p.raw, message)
}

func (p *Private) String() string {
	return p.str
}

func (p *Private) Ed25519() ed25519.PrivateKey {
	return p.raw
}

func (p *Private) Public() string {
	return strings.Split(p.str, ":")[1]
}

func Generate() (public Public, private Private) {
	public.raw, private.raw, _ = ed25519.GenerateKey(rand.Reader)
	public.str = base32enc.EncodeToString([]byte(public.raw))
	private.str = base64enc.EncodeToString(private.raw[:ed25519.PublicKeySize]) + ":" + public.str
	return public, private
}

func ParsePublic(str string) (Public, error) {
	publicBytes, err := base32enc.DecodeString(str)
	if err != nil {
		return Public{}, errors.Wrap(err, "failed to base32 decode")
	}

	if len(publicBytes) != ed25519.PublicKeySize {
		return Public{}, errors.New("invalid key size")
	}

	return Public{
		raw: ed25519.PublicKey(publicBytes),
		str: str,
	}, nil
}

func ParsePrivate(str string) (Private, error) {
	parts := strings.Split(str, ":")
	if len(parts) != 2 {
		return Private{}, errors.New("invalid parts number")
	}

	seed, err := base64enc.DecodeString(parts[0])
	if err != nil {
		return Private{}, errors.New("failed to base64 decode")
	}

	if len(seed) != ed25519.SeedSize {
		return Private{}, errors.New("invalid seed size")
	}

	privateKey := ed25519.NewKeyFromSeed(seed)
	publicBytes, err := base32enc.DecodeString(parts[1])
	if err != nil {
		return Private{}, errors.New("failed to base32 decode")
	}

	if bytes.Compare(privateKey[ed25519.SeedSize:], publicBytes) != 0 {
		return Private{}, errors.New("public part does not match")
	}

	return Private{
		raw: privateKey,
		str: str,
	}, nil
}
