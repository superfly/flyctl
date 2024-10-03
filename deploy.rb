#!/usr/bin/env ruby

require './deploy/common'

event :start, { ts: ts() }

# Change to a directory where we'll pull on git
Dir.chdir("/usr/src/app")

DEPLOY_NOW = !get_env("DEPLOY_NOW").nil?
DEPLOY_CUSTOMIZE = !get_env("NO_DEPLOY_CUSTOMIZE")
DEPLOY_ONLY = !get_env("DEPLOY_ONLY").nil?
CREATE_AND_PUSH_BRANCH = !get_env("DEPLOY_CREATE_AND_PUSH_BRANCH").nil?
FLYIO_BRANCH_NAME = "flyio-new-files"

DEPLOYER_FLY_CONFIG_PATH = get_env("DEPLOYER_FLY_CONFIG_PATH")
DEPLOYER_SOURCE_CWD = get_env("DEPLOYER_SOURCE_CWD")
DEPLOY_APP_NAME = get_env("DEPLOY_APP_NAME")
if !DEPLOY_CUSTOMIZE && !DEPLOY_APP_NAME
  event :error, { type: :validation, message: "missing app name" }
  exit 1
end

DEPLOY_ORG_SLUG = get_env("DEPLOY_ORG_SLUG")
if !DEPLOY_CUSTOMIZE && !DEPLOY_ORG_SLUG
  event :error, { type: :validation, message: "missing organization slug" }
  exit 1
end

DEPLOY_APP_REGION = get_env("DEPLOY_APP_REGION")

DEPLOY_COPY_CONFIG = get_env("DEPLOY_COPY_CONFIG")

GIT_REPO = get_env("GIT_REPO")

CAN_CREATE_AND_PUSH_BRANCH = CREATE_AND_PUSH_BRANCH && GIT_REPO

GIT_REPO_URL = if GIT_REPO
  repo_url = begin
    URI(GIT_REPO)
  rescue StandardError => e
    event :error, { type: :invalid_git_repo_url, message: e }
    exit 1
  end
  if (user = get_env("GIT_URL_USER"))
    repo_url.user = user.strip
  end
  if (password = get_env("GIT_URL_PASSWORD"))
    repo_url.password = password.strip
  end
  repo_url
end

steps = []

# Whatever happens, we try to git pull if a GIT_REPO is specified
steps.push({id: Step::GIT_PULL, description: "Setup and pull from git repository"}) if GIT_REPO

if !DEPLOY_ONLY
  # we're not just deploying, we're also `fly launch`-ing
  steps.push({id: Step::PLAN, description: "Prepare deployment plan"})
  steps.push({id: Step::CUSTOMIZE, description: "Customize deployment plan"}) if DEPLOY_CUSTOMIZE
else
  # only deploying, so we need to send the artifacts right away
  steps.push({id: Step::BUILD, description: "Build image"})
  steps.push({id: Step::DEPLOY, description: "Deploy application"}) if DEPLOY_NOW
  artifact Artifact::META, { steps: steps }
end

if GIT_REPO_URL
    in_step Step::GIT_PULL do
      ref = get_env("GIT_REF")
      artifact Artifact::GIT_INFO, { repository: GIT_REPO, reference: ref }
      
      exec_capture("git init", log: false)

      redacted_repo_url = GIT_REPO_URL.dup
      redacted_repo_url.user = nil
      redacted_repo_url.password = nil

      exec_capture("git remote add origin #{GIT_REPO_URL.to_s}", display: "git remote add origin #{redacted_repo_url.to_s}")

      ref = exec_capture("git remote show origin | sed -n '/HEAD branch/s/.*: //p'", log: false).chomp if !ref

      exec_capture("git -c protocol.version=2 fetch origin #{ref}")
      exec_capture("git reset --hard --recurse-submodules FETCH_HEAD")

      head = JSON.parse(exec_capture("git log -1 --pretty=format:'{\"commit\": \"%H\", \"author\": \"%an\", \"author_email\": \"%ae\", \"date\": \"%ad\", \"message\": \"%f\"}'", log: false))

      artifact Artifact::GIT_HEAD, head

      if !DEPLOYER_SOURCE_CWD.nil?
        Dir.chdir(DEPLOYER_SOURCE_CWD)
      end
    end
end

if !DEPLOYER_FLY_CONFIG_PATH.nil? && !File.exists?(DEPLOYER_FLY_CONFIG_PATH)
  event :error, { type: :validation, message: "Config file #{DEPLOYER_FLY_CONFIG_PATH} does not exist" }
  exit 1
end

FLY_CONFIG_PATH = if !DEPLOYER_FLY_CONFIG_PATH.nil?
  DEPLOYER_FLY_CONFIG_PATH
else
  Dir.entries(".").find { |f| File.fnmatch('fly.{toml,json,yaml,yml}', f, File::FNM_EXTGLOB)}
end
HAS_FLY_CONFIG = !FLY_CONFIG_PATH.nil?

if !DEPLOY_ONLY
  MANIFEST_PATH = "/tmp/manifest.json"

  manifest = in_step Step::PLAN do
    cmd = "flyctl launch plan propose --manifest-path #{MANIFEST_PATH}"

    if (slug = DEPLOY_ORG_SLUG)
      cmd += " --org #{slug}"
    end

    if (name = DEPLOY_APP_NAME)
      cmd += " --force-name --name #{name}"
    end

    if (region = DEPLOY_APP_REGION)
      cmd += " --region #{region}"
    end

    cmd += " --copy-config" if DEPLOY_COPY_CONFIG

    exec_capture(cmd).chomp

    raw_manifest = File.read(MANIFEST_PATH)

    begin
      manifest = JSON.parse(raw_manifest)
    rescue StandardError => e
      event :error, { type: :json, message: e, json: raw_manifest }
      exit 1
    end

    artifact Artifact::MANIFEST, manifest

    manifest
  end

  REQUIRES_DEPENDENCIES = %w[ruby bun node elixir python php]

  RUNTIME_LANGUAGE = manifest.dig("plan", "runtime", "language")
  RUNTIME_VERSION = manifest.dig("plan", "runtime", "version")

  DO_INSTALL_DEPS = REQUIRES_DEPENDENCIES.include?(RUNTIME_LANGUAGE)

  steps.push({id: Step::INSTALL_DEPENDENCIES, description: "Install required dependencies", async: true}) if DO_INSTALL_DEPS

  DEFAULT_ERLANG_VERSION = get_env("DEFAULT_ERLANG_VERSION", "26.2.5.2")

  DEFAULT_RUNTIME_VERSIONS = {
    "ruby"   => get_env("DEFAULT_RUBY_VERSION", "3.1.6"),
    "elixir" => get_env("DEFAULT_ELIXIR_VERSION", "1.16"),
    "erlang" => DEFAULT_ERLANG_VERSION,
    "node" => get_env("DEFAULT_NODE_VERSION", "20.16.0"),
    "bun" => get_env("DEFAULT_BUN_VERSION", "1.1.24"),
    "php" => get_env("DEFAULT_PHP_VERSION", "8.1"),
    "python" => get_env("DEFAULT_PYTHON_VERSION", "3.12")
  }

  ASDF_SUPPORTED_FLYCTL_LANGUAGES = %w[ bun node elixir ]
  FLYCTL_TO_ASDF_PLUGIN_NAME = {
    "node" => "nodejs"
  }

  INSTALLABLE_PHP_VERSIONS = %w[ 5.6 7.0 7.1 7.2 7.3 7.4 8.0 8.1 8.2 8.3 8.4 ]

  deps_thread = Thread.new do
    if DO_INSTALL_DEPS
      in_step Step::INSTALL_DEPENDENCIES do
        # get the version
        version = DEFAULT_RUNTIME_VERSIONS[RUNTIME_LANGUAGE]
        if version.nil?
          event :error, { type: :unsupported_version, message: "unhandled runtime: #{RUNTIME_LANGUAGE}, supported: #{DEFAULT_RUNTIME_VERSIONS.keys.join(", ")}" }
          exit 1
        end

        version = RUNTIME_VERSION.empty? ? version : RUNTIME_VERSION

        if ASDF_SUPPORTED_FLYCTL_LANGUAGES.include?(RUNTIME_LANGUAGE)
          plugin = FLYCTL_TO_ASDF_PLUGIN_NAME.fetch(RUNTIME_LANGUAGE, RUNTIME_LANGUAGE)
          if plugin == "elixir"
            # required for elixir to work
            exec_capture("asdf install erlang #{DEFAULT_ERLANG_VERSION}")  
          end
          exec_capture("asdf install #{plugin} #{version}")
        else
          case RUNTIME_LANGUAGE
          when "ruby"
            exec_capture("rvm install #{version}")
          when "php"
            major, minor = Gem::Version.new(version).segments
            php_version = "#{major}.#{minor}"
            if !INSTALLABLE_PHP_VERSIONS.include?(php_version)
              event :error, { type: :unsupported_version, message: "unsupported PHP version #{version}, supported versions are: #{INSTALLABLE_PHP_VERSIONS.join(", ")}" }
              exit 1
            end
            exec_capture("apt install --no-install-recommends -y php#{php_version} php#{php_version}-curl php#{php_version}-mbstring php#{php_version}-xml")
            exec_capture("curl -sS https://getcomposer.org/installer -o /tmp/composer-setup.php")
            # TODO: verify signature?
            exec_capture("php /tmp/composer-setup.php --install-dir=/usr/local/bin --filename=composer")
          when "python"
            major, minor = Gem::Version.new(version).segments
            python_version = "#{major}.#{minor}"
            exec_capture("mise use -g python@#{python_version}")
          else
            # we should never get here, but handle it in case!
            event :error, { type: :unsupported_version, message: "no handler for runtime: #{RUNTIME_LANGUAGE}, supported: #{DEFAULT_RUNTIME_VERSIONS.keys.join(", ")}" }
            exit 1
          end
        end
      end
    end
  end

  if DEPLOY_CUSTOMIZE
    manifest = in_step Step::CUSTOMIZE do
      cmd = "flyctl launch sessions create --session-path /tmp/session.json --manifest-path #{MANIFEST_PATH} --from-manifest #{MANIFEST_PATH}"

      exec_capture(cmd)
      session = JSON.parse(File.read("/tmp/session.json"))

      artifact Artifact::SESSION, session

      cmd = "flyctl launch sessions finalize --session-path /tmp/session.json --manifest-path #{MANIFEST_PATH}"

      exec_capture(cmd)
      manifest = JSON.parse(File.read("/tmp/manifest.json"))

      artifact Artifact::MANIFEST, manifest

      manifest
    end
  end

  ORG_SLUG = manifest["plan"]["org"]
  APP_REGION = manifest["plan"]["region"]

  DO_GEN_REQS = !DEPLOY_COPY_CONFIG || !HAS_FLY_CONFIG

  debug("generate reqs? #{DO_GEN_REQS}")

  FLY_PG = manifest.dig("plan", "postgres", "fly_postgres")
  SUPABASE = manifest.dig("plan", "postgres", "supabase_postgres")
  UPSTASH = manifest.dig("plan", "redis", "upstash_redis")
  TIGRIS = manifest.dig("plan", "object_storage", "tigris_object_storage")
  SENTRY = manifest.dig("plan", "sentry") == true

  steps.push({id: Step::GENERATE_BUILD_REQUIREMENTS, description: "Generate requirements for build"}) if DO_GEN_REQS
  steps.push({id: Step::BUILD, description: "Build image"})
  steps.push({id: Step::FLY_POSTGRES_CREATE, description: "Create and attach PostgreSQL database"}) if FLY_PG
  steps.push({id: Step::SUPABASE_POSTGRES, description: "Create Supabase PostgreSQL database"}) if SUPABASE
  steps.push({id: Step::UPSTASH_REDIS, description: "Create Upstash Redis database"}) if UPSTASH
  steps.push({id: Step::TIGRIS_OBJECT_STORAGE, description: "Create Tigris object storage bucket"}) if TIGRIS
  steps.push({id: Step::SENTRY, description: "Create Sentry project"}) if SENTRY

  steps.push({id: Step::DEPLOY, description: "Deploy application"}) if DEPLOY_NOW

  if CAN_CREATE_AND_PUSH_BRANCH
    steps.push({id: Step::CREATE_AND_PUSH_BRANCH, description: "Create Fly.io git branch with new files"})
  end

  artifact Artifact::META, { steps: steps }

  # Join the parallel task thread
  deps_thread.join

  if DO_GEN_REQS
    in_step Step::GENERATE_BUILD_REQUIREMENTS do
      exec_capture("flyctl launch plan generate #{MANIFEST_PATH}")
      if GIT_REPO
        exec_capture("git add -A", log: false)
        diff = exec_capture("git diff --cached", log: false)
        artifact Artifact::DIFF, { output: diff }
      end
    end
  end
end

# TODO: better error if missing config
fly_config = manifest && manifest.dig("config") || JSON.parse(exec_capture("flyctl config show --local --config #{FLY_CONFIG_PATH}", log: false))
APP_NAME = DEPLOY_APP_NAME || fly_config["app"]

image_ref = in_step Step::BUILD do
  image_tag = SecureRandom.hex(16)
  if (image_ref = fly_config.dig("build","image")&.strip) && !image_ref.nil? && !image_ref.empty?
    info("Skipping build, using image defined in fly config: #{image_ref}")
    image_ref
  else
    image_ref = "registry.fly.io/#{APP_NAME}:#{image_tag}"

    exec_capture("flyctl deploy --build-only --depot=false --push -a #{APP_NAME} --image-label #{image_tag} --config #{FLY_CONFIG_PATH}")
    artifact Artifact::DOCKER_IMAGE, { ref: image_ref }
    image_ref
  end
end

if !DEPLOY_ONLY && get_env("SKIP_EXTENSIONS").nil?
  if FLY_PG
    in_step Step::FLY_POSTGRES_CREATE do
      pg_name = FLY_PG["app_name"]
      region = APP_REGION

      cmd = "flyctl pg create --flex --org #{ORG_SLUG} --name #{pg_name} --region #{region}"

      if (vm_size = FLY_PG["vm_size"])
        cmd += " --vm-size #{vm_size}"
      end

      if (vm_memory = FLY_PG["vm_ram"])
        cmd += " --vm-memory #{vm_memory}"
      end

      if (nodes = FLY_PG["nodes"])
        cmd += " --initial-cluster-size #{nodes}"
      end

      if (disk_size_gb = FLY_PG["disk_size_gb"])
        cmd += " --volume-size #{disk_size_gb}"
      end

      artifact Artifact::FLY_POSTGRES, { name: pg_name, region: region, config: FLY_PG }

      exec_capture(cmd)

      exec_capture("flyctl pg attach #{pg_name} --app #{APP_NAME} -y")
    end
  elsif SUPABASE
    in_step Step::SUPABASE_POSTGRES do
      cmd = "flyctl ext supabase create --org #{ORG_SLUG} --name #{SUPABASE["db_name"]} --region #{SUPABASE["region"]} --app #{APP_NAME} --yes"

      artifact Artifact::SUPABASE_POSTGRES, { config: SUPABASE }

      exec_capture(cmd)
    end
  end

  if UPSTASH
    in_step Step::UPSTASH_REDIS do
      db_name = "#{APP_NAME}-redis"

      cmd = "flyctl redis create --name #{db_name} --org #{ORG_SLUG} --region #{APP_REGION}"

      if UPSTASH["eviction"] == true
        cmd += " --enable-eviction"
      elsif UPSTASH["eviction"] == false
        cmd += " --disable-eviction"
      end

      if (regions = UPSTASH["regions"])
        cmd += " --replica-regions #{regions.join(",")}"
      end

      artifact Artifact::UPSTASH_REDIS, { config: UPSTASH, region: APP_REGION, name: db_name }

      exec_capture(cmd)
    end
  end

  if TIGRIS
    in_step Step::TIGRIS_OBJECT_STORAGE do
      cmd = "flyctl ext tigris create --org #{ORG_SLUG} --app #{APP_NAME} --yes"

      if (name = TIGRIS["name"]) && !name.empty?
        cmd += " --name #{name}"
      end

      if (pub = TIGRIS["public"]) && pub == true
        cmd += " --public"
      end

      if (accel = TIGRIS["accelerate"]) && accel == true
        cmd += " --accelerate"
      end

      if (domain = TIGRIS["website_domain_name"]) && !domain.empty?
        cmd += " --website-domain-name #{domain}"
      end

      artifact Artifact::TIGRIS_OBJECT_STORAGE, { config: TIGRIS }

      exec_capture(cmd)
    end
  end

  if SENTRY
    in_step Step::SENTRY do
      exec_capture("flyctl ext sentry create --app #{APP_NAME} --yes")
    end
  end
end

if DEPLOY_NOW
  in_step Step::DEPLOY do
    exec_capture("flyctl deploy -a #{APP_NAME} --image #{image_ref} --config #{FLY_CONFIG_PATH}")
  end
end

if CAN_CREATE_AND_PUSH_BRANCH
  in_step Step::CREATE_AND_PUSH_BRANCH do
    exec_capture("git checkout -b #{FLYIO_BRANCH_NAME}")
    exec_capture("git config user.name \"Fly.io\"")
    exec_capture("git config user.email \"noreply@fly.io\"")
    exec_capture("git add .")
    exec_capture("git commit -m \"New files from Fly.io Launch\" || echo \"No changes to commit\"")
    exec_capture("git push -f origin #{FLYIO_BRANCH_NAME}")
  end
end

event :end, { ts: ts() }