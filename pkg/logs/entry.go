package logs

type LogEntry struct {
	Level     string `json:"level"`
	Instance  string `json:"instance"`
	Message   string `json:"message"`
	Region    string `json:"region"`
	Timestamp string `json:"timestamp"`
	Meta      Meta   `json:"meta"`
}

type Meta struct {
	Instance string
	Region   string
	Event    struct {
		Provider string
	}
	HTTP struct {
		Request struct {
			ID      string
			Method  string
			Version string
		}
		Response struct {
			StatusCode int `json:"status_code"`
		}
	}
	Error struct {
		Code    int
		Message string
	}
	URL struct {
		Full string
	}
}

type natsLog struct {
	Event struct {
		Provider string `json:"provider"`
	} `json:"event"`
	Fly struct {
		App struct {
			Instance string `json:"instance"`
			Name     string `json:"name"`
		} `json:"app"`
		Region string `json:"region"`
	} `json:"fly"`
	Host string `json:"host"`
	Log  struct {
		Level string `json:"level"`
	} `json:"log"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

// func NewLogStream(apiClient *api.Client) (*LogStream, error) {
// 	return &LogStream{apiClient: apiClient}, nil
// }

// func (ls *LogStream) Stream(ctx context.Context, opts *LogOptions) <-chan Entry {
// 	app, err := ls.apiClient.GetApp(opts.AppName)
// 	if err != nil {
// 		ls.err = err
// 		return nil
// 	}

// 	dialer, err := func() (agent.Dialer, error) {
// 		agentclient, err := agent.Establish(ctx, ls.apiClient)
// 		if err != nil {
// 			return nil, errors.Wrap(err, "error establishing agent")
// 		}

// 		dialer, err := agentclient.Dialer(ctx, &app.Organization)
// 		if err != nil {
// 			return nil, errors.Wrapf(err, "error establishing wireguard connection for %s organization", app.Organization.Slug)
// 		}

// 		tunnelCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
// 		defer cancel()
// 		if err = agentclient.WaitForTunnel(tunnelCtx, &app.Organization); err != nil {
// 			return nil, errors.Wrap(err, "unable to connect WireGuard tunnel")
// 		}

// 		return dialer, nil
// 	}()

// 	if err != nil {
// 		terminal.Debugf("could not connect to wireguard tunnel, err: %v\n", err)
// 		terminal.Debug("Falling back to log polling...")

// 		return ls.poll(ctx, opts)

// 	}

// 	terminal.Debug("Streaming logs from nats...")

// 	return ls.nats(ctx, opts, dialer)
// }

// func (ls *LogStream) Err() error {
// 	return ls.err
// }

// func (ls *LogStream) poll(ctx context.Context, opts *LogOptions) <-chan Entry {
// 	out := make(chan Entry)

// 	b := &backoff.Backoff{
// 		Min:    250 * time.Millisecond,
// 		Max:    5 * time.Second,
// 		Factor: 2,
// 		Jitter: true,
// 	}

// 	if opts.MaxBackoff != 0 {
// 		b.Max = opts.MaxBackoff
// 	}

// 	go func() {
// 		defer close(out)
// 		errorCount := 0
// 		nextToken := ""

// 		var wait <-chan time.Time

// 		for {
// 			entries, token, err := ls.apiClient.GetAppLogs(opts.AppName, nextToken, opts.RegionCode, opts.VMID)

// 			if err != nil {
// 				errorCount++

// 				if api.IsNotAuthenticatedError(err) || api.IsNotFoundError(err) || errorCount > 10 {
// 					ls.err = err
// 					return
// 				}
// 				wait = time.After(b.Duration())
// 			} else {
// 				errorCount = 0

// 				if len(entries) == 0 {
// 					wait = time.After(b.Duration())
// 				} else {
// 					b.Reset()

// 					for _, item := range entries {
// 						out <- Entry{
// 							Level:     item.Level,
// 							Message:   item.Message,
// 							Region:    item.Region,
// 							Timestamp: item.Timestamp,
// 							Instance:  item.Instance,
// 							Meta: Meta{
// 								Event: struct{ Provider string }{
// 									Provider: item.Meta.Event.Provider,
// 								},
// 							},
// 						}
// 					}
// 					wait = time.After(0)

// 					if token != "" {
// 						nextToken = token
// 					}
// 				}
// 			}

// 			select {
// 			case <-ctx.Done():
// 				return
// 			case <-wait:
// 			}
// 		}
// 	}()

// 	return out
// }

// func (ls *LogStream) nats(ctx context.Context, opts *LogOptions, dialer agent.Dialer) <-chan Entry {
// 	out := make(chan Entry)

// 	app, err := ls.apiClient.GetApp(opts.AppName)
// 	if err != nil {
// 		ls.err = err
// 		return nil
// 	}

// 	conn, err := func() (*nats.Conn, error) {
// 		var flyConf flyConfig
// 		usr, _ := user.Current()
// 		flyConfFile, err := os.Open(filepath.Join(usr.HomeDir, ".fly", "config.yml"))
// 		if err != nil {
// 			return nil, errors.Wrap(err, "could not read fly config yml")

// 		}

// 		if err := yaml.NewDecoder(flyConfFile).Decode(&flyConf); err != nil {
// 			return nil, errors.Wrap(err, "could not decode fly config yml")
// 		}

// 		state, ok := flyConf.WireGuardState[app.Organization.Slug]
// 		if !ok {
// 			return nil, errors.New("could not find org in fly config")
// 		}

// 		peerIP := state.Peer.PeerIP

// 		var natsIPBytes [16]byte
// 		copy(natsIPBytes[0:], peerIP[0:6])
// 		natsIPBytes[15] = 3

// 		natsIP := net.IP(natsIPBytes[:])

// 		conn, err := nats.Connect(fmt.Sprintf("nats://[%s]:4223", natsIP.String()), nats.SetCustomDialer(&natsDialer{dialer, ctx}), nats.UserInfo(app.Organization.Slug, flyConf.AccessToken))
// 		if err != nil {
// 			return nil, errors.Wrap(err, "could not connect to nats")
// 		}
// 		return conn, nil
// 	}()

// 	go func() {
// 		subject := fmt.Sprintf("logs.%s", opts.AppName)

// 		if opts.RegionCode != "" {
// 			subject = fmt.Sprintf("%s.%s", subject, opts.RegionCode)
// 		} else {
// 			subject = fmt.Sprintf("%s.%s", subject, "*")
// 		}

// 		if opts.VMID != "" {
// 			subject = fmt.Sprintf("%s.%s", subject, opts.VMID)
// 		} else {
// 			subject = fmt.Sprintf("%s.%s", subject, "*")
// 		}

// 		subject = fmt.Sprintf("%s.%s", subject, ">")

// 		sub, err := conn.Subscribe(subject, func(msg *nats.Msg) {
// 			var log natsLog
// 			if err := json.Unmarshal(msg.Data, &log); err != nil {
// 				terminal.Error(errors.Wrap(err, "could not parse log"))
// 				return
// 			}
// 			// send
// 			out <- Entry{
// 				Instance:  log.Fly.App.Instance,
// 				Level:     log.Log.Level,
// 				Message:   log.Message,
// 				Region:    log.Fly.Region,
// 				Timestamp: log.Timestamp,
// 				Meta: Meta{
// 					Event: struct {
// 						Provider string
// 					}{
// 						Provider: log.Event.Provider,
// 					},
// 				},
// 			}
// 		})
// 		if err != nil {
// 			ls.err = errors.Wrap(err, "could not sub to logs via nats")
// 		}

// 		<-ctx.Done()

// 		defer sub.Unsubscribe()
// 	}()

// 	return out
// }
