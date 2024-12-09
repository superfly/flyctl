# Hello Elixir SQLite!

Welcome to our Code Server for Phoenix Apps.

## Development

Right now this editor is running at ${FLY_CODE_URL}. 

You need to start the development server to see yout app running at ${FLY_DEVELOPMENT_URL}.

```sh
mix phx.server
```

## Deploy

Looks like we're ready to deploy!

To deploy you just need to run `fly launch --no-deploy`, create your secret key and create a volume. 

Run `fly launch --no-deploy` and make sure to say yes to copy the configuration file 
to the new app so you wont have to do anything.

```sh
$ fly launch --no-deploy
An existing fly.toml file was found for app fly-elixir

? Would you like to copy its configuration to the new app? Yes
Creating app in /home/coder/project
Scanning source code
Detected a Dockerfile app

? App Name (leave blank to use an auto-generated name): your-app-name

? Select organization: Lubien (personal)

? Select region: gru (São Paulo)

Created app sqlite-tests in organization personal
Wrote config file fly.toml
Your app is ready. Deploy with `flyctl deploy`
```

Let's got create your secret key. Elixir has a mix task that can generate a new 
Phoenix key base secret. Let's use that.

```bash
mix phx.gen.secret
```

It generates a long string of random text. Let's store that as a secret for our app. 
When we run this command in our project folder, `flyctl` uses the `fly.toml` 
file to know which app we are setting the value on.

```sh
$ fly secrets set SECRET_KEY_BASE=<GENERATED>
Secrets are staged for the first deployment
```

Now time to create a volume for your SQLite database. You will need to run
`fly volumes create database_data --region REGION_NAME`. Pick the same region
you chose on the previous command.

```sh
$ fly volumes create database_data --size 1 --region gru
        ID: vol_1g67340g9y9rydxw
      Name: database_data
       App: sqlite-tests
    Region: gru
      Zone: 2824
   Size GB: 1
 Encrypted: true
Created at: 18 Jan 22 11:18 UTC
```

Now go for the deploy!

```sh
$ fly deploy
```

... will bring up your app!