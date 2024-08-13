#!/usr/bin/ruby

require 'json'
require 'time'
require 'open3'
require 'uri'
require 'securerandom'
require 'fileutils'

LOG_PREFIX = ENV["LOG_PREFIX"]

module Step
  ROOT = :__root__
  GIT_PULL = :git_pull
  PLAN = :plan
  CUSTOMIZE = :customize
  BUILD = :build
  FLY_POSTGRES_CREATE = :fly_postgres_create
  SUPABASE_POSTGRES = :supabase_postgres
  UPSTASH_REDIS = :upstash_redis
  TIGRIS_OBJECT_STORAGE = :tigris_object_storage
  SENTRY = :sentry
  DEPLOY = :deploy
end

module Artifact
  META = :meta
  GIT_INFO = :git_info
  GIT_HEAD = :git_head
  MANIFEST = :manifest
  SESSION = :session
  DIFF = :diff
  FLY_POSTGRES = :fly_postgres
  SUPABASE_POSTGRES = :supabase_postgres
  UPSTASH_REDIS = :upstash_redis
  TIGRIS_OBJECT_STORAGE = :tigris_object_storage
  SENTRY = :sentry
  DOCKER_IMAGE = :docker_image
end

$current_step = Step::ROOT

$counter = 0
$counter_mutex = Mutex.new

def id
  $counter_mutex.synchronize do
    $counter += 1
    $counter
  end
end

$start = Process.clock_gettime(Process::CLOCK_MONOTONIC)

def elapsed
  Process.clock_gettime(Process::CLOCK_MONOTONIC) - $start
end

def nputs(type:, payload: nil)
  obj = { id: id(), step: $current_step, type: type, time: elapsed(), payload: payload }.compact
  puts "#{LOG_PREFIX}#{obj.to_json}"
end

# prefixed events
def event(name, meta = nil)
  nputs(type: "event:#{name}", payload: meta)
end

def artifact(name, body)
  nputs(type: "artifact:#{name}", payload: body)
end

def log(level, msg)
  nputs(type: "log:#{level}", payload: msg)
end

def info(msg)
    log("info", msg)
end

def debug(msg)
    log("debug", msg)
end

def error(msg)
    log("error", msg)
end

def exec_capture(cmd, display = nil)
    event :exec, { cmd: display || cmd }

    out_mutex = Mutex.new
    output = ""

    status = Open3.popen3(cmd) do |stdin, stdout, stderr, wait_thr|
        pid = wait_thr.pid

        stdin.close_write

        threads = [[stdout, "stdout"], [stderr, "stderr"]].map do |stream, stream_name|
          Thread.new do
            stream.each_line do |line|
              nputs type: stream_name, payload: line.chomp
              out_mutex.synchronize { output += line }
            end
          end
        end

        threads.each { |thr| thr.join }

        wait_thr.value
    end

    if !status.success?
        event :error, { type: :exec, message: "unsuccessful command '#{cmd}'", exit_code: status.exitstatus, pid: status.pid }
        exit 1
    end

    output
end

def in_step(step, &block)
  old_step = $current_step
  $current_step = step
  event :start
  ret = begin
    yield block
  rescue StandardError => e
    event :error, { type: :uncaught, message: e }
    exit 1
  end
  event :end
  $current_step = old_step
  ret
end

def ts
  Time.now.utc.iso8601(6)
end

def get_env(name)
  value = ENV[name]&.strip
  if value.nil? || value.empty?
    return nil
  end
  value
end

# start of actual logic

event :start, { ts: ts() }

ORG_SLUG = get_env("DEPLOY_ORG_SLUG")
if !ORG_SLUG
  event :error, { type: :validation, message: "missing organization slug" }
  exit 1
end

GIT_REPO = get_env("GIT_REPO")

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

DEPLOY_NOW = !get_env("DEPLOY_NOW").nil?

steps = []

steps.push({id: Step::GIT_PULL, description: "Setup and pull from git repository"}) if GIT_REPO
steps.push({id: Step::PLAN, description: "Prepare deployment plan"})
steps.push({id: Step::CUSTOMIZE, description: "Customize deployment plan"})
steps.push({id: Step::BUILD, description: "Build image"}) if GIT_REPO

APP_REGION = get_env("DEPLOY_APP_REGION")

if GIT_REPO_URL
    in_step Step::GIT_PULL do
      `git config --global init.defaultBranch main` # NOTE: this is to avoid a large warning message
      ref = get_env("GIT_REF")
      artifact Artifact::GIT_INFO, { repository: GIT_REPO, reference: ref }
      
      exec_capture("git init")

      redacted_repo_url = GIT_REPO_URL.dup
      redacted_repo_url.user = nil
      redacted_repo_url.password = nil

      exec_capture("git remote add origin #{GIT_REPO_URL.to_s}", "git remote add origin #{redacted_repo_url.to_s}")

      ref = exec_capture("git remote show origin | sed -n '/HEAD branch/s/.*: //p'").chomp if !ref

      exec_capture("git -c protocol.version=2 fetch origin #{ref}")
      exec_capture("git reset --hard --recurse-submodules FETCH_HEAD")

      head = JSON.parse(exec_capture("git log -1 --pretty=format:'{\"commit\": \"%H\", \"author\": \"%an\", \"author_email\": \"%ae\", \"date\": \"%ad\", \"message\": \"%f\"}'"))

      artifact Artifact::GIT_HEAD, head
    end
end

session = in_step Step::PLAN do
  cmd = "flyctl launch sessions create --session-path /tmp/session.json --manifest-path /tmp/manifest.json"
  
  if (region = APP_REGION)
    cmd += " --region #{region}"
  end
  
  if (internal_port = get_env("DEPLOY_APP_INTERNAL_PORT"))
    cmd += " --internal-port #{internal_port}"
  end

  cmd += " --copy-config" if get_env("DEPLOY_COPY_CONFIG")

  exec_capture(cmd)
  session = JSON.parse(File.read("/tmp/session.json"))
  manifest = JSON.parse(File.read("/tmp/manifest.json"))

  artifact Artifact::MANIFEST, manifest
  artifact Artifact::SESSION, session

  exec_capture("git add -A")
  diff = exec_capture("git diff --cached")
  artifact Artifact::DIFF, diff

  session
end

manifest = in_step Step::CUSTOMIZE do
  cmd = "flyctl launch sessions finalize --session-path /tmp/session.json --manifest-path /tmp/manifest.json"

  exec_capture(cmd)
  manifest = JSON.parse(File.read("/tmp/manifest.json"))

  artifact Artifact::MANIFEST, manifest

  manifest
end

# Write the fly config file to a tmp directory
File.write("/tmp/fly.json", manifest["config"].to_json)

APP_NAME = manifest["config"]["app"]

FLY_PG = manifest.dig("plan", "postgres", "fly_postgres")
SUPABASE = manifest.dig("plan", "postgres", "supabase_postgres")
UPSTASH = manifest.dig("plan", "redis", "upstash_redis")
TIGRIS = manifest.dig("plan", "object_storage", "tigris_object_storage")
SENTRY = manifest.dig("plan", "sentry") == true

steps.push({id: Step::FLY_POSTGRES_CREATE, description: "Create and attach PostgreSQL database"}) if FLY_PG
steps.push({id: Step::SUPABASE_POSTGRES, description: "Create Supabase PostgreSQL database"}) if SUPABASE
steps.push({id: Step::UPSTASH_REDIS, description: "Create Upstash Redis database"}) if UPSTASH
steps.push({id: Step::TIGRIS_OBJECT_STORAGE, description: "Create Tigris object storage bucket"}) if TIGRIS
steps.push({id: Step::SENTRY, description: "Create Sentry project"}) if SENTRY

steps.push({id: Step::DEPLOY, description: "Deploy application"}) if DEPLOY_NOW

artifact Artifact::META, { steps: steps }

image_tag = SecureRandom.hex(16)

image_ref = in_step Step::BUILD do
  if (image_ref = manifest.dig("config","build","image")&.strip) && !image_ref.nil? && !image_ref.empty?
    info("Skipping build, using image defined in fly config: #{image_ref}")
    image_ref
  else
    image_ref = "registry.fly.io/#{APP_NAME}:#{image_tag}"

    exec_capture("flyctl deploy -a #{APP_NAME} -c /tmp/fly.json --build-only --push --image-label #{image_tag}")
    artifact Artifact::DOCKER_IMAGE, image_ref
    image_ref
  end
end

if FLY_PG
  in_step Step::FLY_POSTGRES_CREATE do
    pg_name = FLY_PG["app_name"]
    region = APP_REGION

    cmd = "flyctl pg create --flex --org #{ORG_SLUG} --name #{pg_name} --region #{region} --yes"

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

    cmd = "flyctl redis create --name #{db_name} --org #{ORG_SLUG} --region #{APP_REGION} --yes"

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

if DEPLOY_NOW
  in_step Step::DEPLOY do
    exec_capture("flyctl deploy -a #{APP_NAME} -c /tmp/fly.json --image #{image_ref}")
  end
end

event :end, { ts: ts() }