# Blue-Green Deploy: Silent Health Check Bypass on Stopped Machines

## What happens

When you run `fly deploy --strategy bluegreen` and your existing machines are **stopped** (e.g. auto-stopped due to no traffic), the deploy:

1. Reports all green machines as **started** — they were never started
2. Reports all health checks as **1/1 passing** — no check was ever run
3. Destroys the old machines
4. Leaves you with a broken app and zero traffic served

No error. No warning. Just "Deployment Complete."

## When does it trigger

Any time blue machines are not in `started`, `starting`, or `failed` state when a deploy kicks off. The most common scenario:

- `auto_stop_machines = 'stop'` with `min_machines_running = 0` (a very common, recommended config for low-traffic apps)
- A period of zero traffic stops all machines
- Developer deploys — the whole point of blue-green was to catch regressions safely

This is exactly the use case blue-green exists for, and it's exactly when the safety net silently disappears.

## Why it's bad

Blue-green's entire value proposition is **"don't cut over until the new version is proven healthy."** When this bug fires, users get the opposite: a false sense of safety. The deploy looks successful, CI goes green, and the app is down. The user has to go investigate `fly machines list` or check the monitoring dashboard to discover what happened — nothing in the deploy output hints that anything went wrong.
