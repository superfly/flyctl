package wg

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net"

	"golang.zx2c4.com/wireguard/device"
)

type Config struct {
	LocalPrivateKey PrivateKey `toml:"local_private_key"`
	LocalNetwork    *IPNet     `toml:"local_network"`

	RemotePublicKey PublicKey `toml:"remote_public_key"`
	RemoteNetwork   *IPNet    `toml:"remote_network"`

	Endpoint  string `toml:"endpoint"`
	DNS       net.IP `toml:"dns"`
	KeepAlive int    `toml:"keepalive"`
	MTU       int    `toml:"mtu"`
	LogLevel  int    `toml:"log_level"`
}

type IPNet net.IPNet

func (n *IPNet) MarshalText() ([]byte, error) {
	return []byte((*net.IPNet)(n).String()), nil
}

func (n *IPNet) UnmarshalText(text []byte) error {
	_, ipnet, err := net.ParseCIDR(string(text))
	if err != nil {
		return err
	}

	n.IP, n.Mask = ipnet.IP, ipnet.Mask
	return nil
}

func (n *IPNet) String() string { return (*net.IPNet)(n).String() }

type PrivateKey device.NoisePrivateKey

func (pk PrivateKey) MarshalText() ([]byte, error) {
	return []byte(base64.StdEncoding.EncodeToString(pk[:])), nil
}

func (pk *PrivateKey) UnmarshalText(text []byte) error {
	buf, err := base64.StdEncoding.DecodeString(string(text))
	if err != nil {
		return err
	}
	if len(buf) != device.NoisePrivateKeySize {
		return errors.New("invalid noise private key")
	}

	copy(pk[:], buf)
	return nil
}

func (pk PrivateKey) ToHex() string {
	val := (device.NoisePrivateKey)(pk)
	return hex.EncodeToString(val[:])
}

type PublicKey device.NoisePublicKey

func (pk PublicKey) MarshalText() ([]byte, error) {
	return []byte(base64.StdEncoding.EncodeToString(pk[:])), nil
}

func (pk *PublicKey) UnmarshalText(text []byte) error {
	buf, err := base64.StdEncoding.DecodeString(string(text))
	if err != nil {
		return err
	}
	if len(buf) != device.NoisePublicKeySize {
		return errors.New("invalid noise private key")
	}

	copy(pk[:], buf)
	return nil
}

func (pk PublicKey) ToHex() string {
	val := (device.NoisePublicKey)(pk)
	return hex.EncodeToString(val[:])
}
