package nats

import (
	"github.com/nats-io/nats.go"
)

type Options struct {
	Password string
	Token    string
	Dialer   nats.CustomDialer
}

type Client struct {
	*nats.Conn
}

func Connect(url string, opts *Options) (*nats.Conn, error) {
	// var natOpts []nats.Option

	// if opts != nil {
	// 	if opts.Dialer != nil {
	// 		natOpts = append(natOpts, nats.CustomDialer(opts.Dialer))
	// 	}
	// 	if opts.Password != "" && opts.Token != "" {
	// 		natOpts = append(natOpts, nats.UserInfo(opts.Pass, flyConf.AccessToken))
	// 	}
	// }

	// conn, err := nats.Connect(fmt.Sprintf("nats://[%s]:4223", natsIP.String()), nats.SetCustomDialer(&natsDialer{dialer, ctx}), nats.UserInfo(app.Organization.Slug, flyConf.AccessToken))
	// if err != nil {
	// 	return errors.Wrap(err, "could not connect to nats")
	// }

	return nil, nil
}
