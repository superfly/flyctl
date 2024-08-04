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
  BUILD = :build
  FLY_POSTGRES_CREATE = :fly_postgres_create
  DEPLOY = :deploy
end

module Artifact
  META = :meta
  GIT_INFO = :git_info
  GIT_HEAD = :git_head
  MANIFEST = :manifest
  DIFF = :diff
  FLY_POSTGRES = :fly_postgres
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

def exec_capture(cmd)
    event :exec, { cmd: cmd }

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

APP_NAME = get_env("DEPLOY_APP_NAME")
if !APP_NAME
  event :error, { type: :validation, message: "missing app name" }
  exit 1
end

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

PG_PROVIDER = get_env("DEPLOY_PG_PROVIDER")
FLY_PG_PROVIDER = PG_PROVIDER == "fly_postgres"

PG_NAME = get_env("DEPLOY_PG_NAME")
PG_FLY_CONFIG = get_env("DEPLOY_PG_FLY_CONFIG")
PG_REGION = get_env("DEPLOY_PG_REGION")

if FLY_PG_PROVIDER
  if !PG_FLY_CONFIG
    event :error, { type: :validation, message: "Missing DEPLOY_PG_FLY_CONFIG" }
    exit 1
  end
end

DEPLOY_NOW = !get_env("DEPLOY_NOW").nil?

steps = []

steps.push({id: Step::GIT_PULL, description: "Setup and pull from git repository"}) if GIT_REPO
steps.push({id: Step::PLAN, description: "Plan deployment"})
steps.push({id: Step::BUILD, description: "Build image"}) if GIT_REPO
steps.push({id: Step::FLY_POSTGRES_CREATE, description: "Create and attach PostgreSQL database"}) if FLY_PG_PROVIDER
steps.push({id: Step::DEPLOY, description: "Deploy application"}) if DEPLOY_NOW

artifact Artifact::META, { steps: steps }

APP_REGION = get_env("DEPLOY_APP_REGION")

if GIT_REPO_URL
    in_step Step::GIT_PULL do
      `git config --global init.defaultBranch main` # NOTE: this is to avoid a large warning message
      ref = get_env("GIT_REF")
      artifact Artifact::GIT_INFO, { repository: GIT_REPO, reference: ref }
      
      exec_capture("git init")

      exec_capture("git remote add origin #{GIT_REPO_URL.to_s}")

      ref = exec_capture("git remote show origin | sed -n '/HEAD branch/s/.*: //p'").chomp if !ref

      exec_capture("git -c protocol.version=2 fetch origin #{ref}")
      exec_capture("git reset --hard --recurse-submodules FETCH_HEAD")

      head = JSON.parse(exec_capture("git log -1 --pretty=format:'{\"commit\": \"%H\", \"author\": \"%an\", \"author_email\": \"%ae\", \"date\": \"%ad\", \"message\": \"%f\"}'"))

      artifact Artifact::GIT_HEAD, head
    end
end

manifest = in_step Step::PLAN do
  cmd = "flyctl launch generate -a #{APP_NAME} -o #{ORG_SLUG} --manifest-path /tmp/manifest.json"
  
  if (region = APP_REGION)
    cmd += " --region #{region}"
  end
  
  if (internal_port = get_env("DEPLOY_APP_INTERNAL_PORT"))
    cmd += " --internal-port #{internal_port}"
  end

  cmd += " --copy-config" if get_env("DEPLOY_COPY_CONFIG")

  exec_capture(cmd)
  manifest = JSON.parse(File.read("/tmp/manifest.json"))

  vm_cpu_kind = ENV.fetch("DEPLOY_VM_CPU_KIND", manifest["plan"]["vm_cpu_kind"])
  vm_cpus = ENV.fetch("DEPLOY_VM_CPUS", manifest["plan"]["vm_cpus"])
  vm_memory = ENV.fetch("DEPLOY_VM_MEMORY", manifest["plan"]["vm_memory"])
  vm_size = ENV.fetch("DEPLOY_VM_SIZE", manifest["plan"]["vm_size"])

  # override this to be sure...
  manifest["config"]["vm"] = [{
    size: vm_size,
    memory: vm_memory,
    cpu_kind: vm_cpu_kind,
    cpus: vm_cpus.to_i
  }]

  artifact Artifact::MANIFEST, manifest

  exec_capture("git add -A")
  diff = exec_capture("git diff --cached")
  artifact Artifact::DIFF, diff

  manifest
end

# Write the fly config file to a tmp directory
File.write("/tmp/fly.json", manifest["config"].to_json)

image_tag = SecureRandom.hex(16)

image_ref = in_step Step::BUILD do
  if (image_ref = manifest.dig("config","build","image")&.strip) && !image_ref.nil? && !image_ref.empty?
    info("Skipping build, using image defined in fly config: #{image_ref}")
    return image_ref
  end

  image_ref = "registry.fly.io/#{APP_NAME}:#{image_tag}"

  exec_capture("flyctl deploy -a #{APP_NAME} -c /tmp/fly.json --build-only --push --image-label #{image_tag}")
  artifact Artifact::DOCKER_IMAGE, image_ref
  image_ref
end

if FLY_PG_PROVIDER
  in_step Step::FLY_POSTGRES_CREATE do
    cmd = "flyctl pg create --flex --config-name #{PG_FLY_CONFIG} --org #{ORG_SLUG}"
  

    pg_name = PG_NAME
    pg_name ||= "#{APP_NAME}-db-#{SecureRandom.hex(2)}"
    
    cmd += " --name #{pg_name}"

    region = PG_REGION
    region ||= APP_REGION

    cmd += " --region #{region}" if region

    artifact Artifact::FLY_POSTGRES, { name: pg_name, region: region, config: PG_FLY_CONFIG }

    exec_capture(cmd)

    exec_capture("flyctl pg attach #{pg_name} --app #{APP_NAME} -y")
  end
end

if DEPLOY_NOW
  in_step Step::DEPLOY do
    exec_capture("flyctl deploy -a #{APP_NAME} -c /tmp/fly.json --image #{image_ref}")
  end
end

event :end, { ts: ts() }