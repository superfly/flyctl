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
  INSTALL_DEPENDENCIES = :install_dependencies
  GENERATE_BUILD_REQUIREMENTS = :generate_build_requirements
  BUILD = :build
  FLY_POSTGRES_CREATE = :fly_postgres_create
  SUPABASE_POSTGRES = :supabase_postgres
  UPSTASH_REDIS = :upstash_redis
  TIGRIS_OBJECT_STORAGE = :tigris_object_storage
  SENTRY = :sentry
  CREATE_AND_PUSH_BRANCH = :create_and_push_branch
  DEPLOY = :deploy

  def self.current
    Thread.current[:step] ||= Step::ROOT
  end

  def self.set_current(step)
    Thread.current[:step] = step
  end
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
  obj = { id: id(), step: Step.current(), type: type, time: elapsed(), payload: payload }.compact
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

def exec_capture(cmd, display: nil, log: true)
  cmd_display = display || cmd
  event :exec, { cmd: cmd_display }

  out_mutex = Mutex.new
  output = ""

  status = Open3.popen3("/bin/bash", "-lc", cmd) do |stdin, stdout, stderr, wait_thr|
      pid = wait_thr.pid

      stdin.close_write

      step = Step.current

      threads = [[stdout, "stdout"], [stderr, "stderr"]].map do |stream, stream_name|
        Thread.new do
          Step.set_current(step) # in_step would be a problem here, we just need to that the thread with the parent thread's step!
          stream.each_line do |line|
            if log
              nputs type: stream_name, payload: line.chomp
            end
            out_mutex.synchronize { output += line }
          end
        end
      end

      threads.each { |thr| thr.join }

      wait_thr.value
  end

  if !status.success?
      event :error, { type: :exec, message: "unsuccessful command '#{cmd_display}'", exit_code: status.exitstatus, pid: status.pid }
      exit 1
  end

  output
end

def in_step(step, &block)
  old_step = Step.current()
  Step.set_current(step)
  event :start
  ret = begin
    yield block
  rescue StandardError => e
    event :error, { type: :uncaught, message: e }
    exit 1
  end
  event :end
  Step.set_current(old_step)
  ret
end

def ts
  Time.now.utc.iso8601(6)
end

def get_env(name, default = nil)
  value = ENV[name]&.strip
  if value.nil? || value.empty?
    return nil || default
  end
  value
end
