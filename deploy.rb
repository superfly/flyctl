#!/usr/bin/ruby

require 'json'
require 'time'
require 'open3'

LOG_PREFIX = ENV["LOG_PREFIX"]

module Step
  ROOT = :__root__
  GIT_PULL = :git_pull
  PLAN = :plan
  DEPLOY = :deploy
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

event :start, { ts: ts() }

APP_NAME = ENV["DEPLOY_APP_NAME"]
if !APP_NAME
  event :error, { type: :validation, message: "missing app name" }
  exit 1
end

ORG_SLUG = ENV["DEPLOY_ORG_SLUG"]
if !ORG_SLUG
  event :error, { type: :validation, message: "missing organization slug" }
  exit 1
end

if (git_repo = ENV["GIT_REPO"]) && !!git_repo
    in_step Step::GIT_PULL do
      `git config --global init.defaultBranch main`
      ref = ENV["GIT_REF"]
      artifact :git_info, { repository: git_repo, reference: ref }
      exec_capture("git init")
      exec_capture("git remote add origin #{git_repo}")
      ref = exec_capture("git remote show origin | sed -n '/HEAD branch/s/.*: //p'").chomp if !ref
      exec_capture("git -c protocol.version=2 fetch origin #{ref}")
      exec_capture("git reset --hard --recurse-submodules FETCH_HEAD")
      head = JSON.parse(exec_capture("git log -1 --pretty=format:'{\"commit\": \"%H\", \"author\": \"%an\", \"author_email\": \"%ae\", \"date\": \"%ad\", \"message\": \"%f\"}'"))
      artifact :git_head, head
    end
end

manifest = in_step Step::PLAN do
  exec_capture("flyctl launch generate -a #{APP_NAME} -o #{ORG_SLUG} --manifest-path /tmp/manifest.json")
  manifest = JSON.parse(File.read("/tmp/manifest.json"))
  artifact :manifest, manifest
  manifest
end

if ENV["DEPLOY_NOW"]
  in_step Step::DEPLOY do
    vm_cpukind = manifest["plan"]["vm_cpukind"]
    vm_cpus = manifest["plan"]["vm_cpus"]
    vm_memory = manifest["plan"]["vm_memory"]
    vm_size = manifest["plan"]["vm_size"]

    File.write("/tmp/fly.json", manifest["config"].to_json)

    exec_capture("flyctl deploy -a #{APP_NAME} --vm-cpu-kind #{vm_cpukind} --vm-cpus #{vm_cpus} --vm-memory #{vm_memory} --vm-size #{vm_size} -c /tmp/fly.json")
  end
end

event :end, { ts: ts() }