// Type declarations for the flyssh wasm module (built from cmd/wasm).
//
// Loading flyssh.wasm through Go's wasm_exec.js and calling go.run() installs
// a `flySSH` global; this file declares its shape. It is the reference for
// the API: the Go entry points in cmd/wasm each name the signature they
// implement here. Include it with a `/// <reference path="flyssh.d.ts" />`
// or through `types` in tsconfig.
//
// Every call that does network I/O returns a Promise and runs on its own
// goroutine. Callbacks are invoked on the JavaScript event loop and must
// not block; they may return a Promise where noted.

/** The module's global, present once go.run() has started the module. */
declare const flySSH: FlySSH;

interface FlySSH {
  /**
   * Generates an ed25519 SSH keypair inside the page, so the private key
   * never leaves it: hand `publicKey` to whatever issues the certificate and
   * pass `privateKey` and the certificate to connect().
   */
  generateKey(): Promise<KeyPair>;

  /**
   * Brings up a WireGuard tunnel to a token-mode gateway and opens an SSH
   * connection to `host` through it. Rejects if the gateway refuses the
   * token, the host doesn't resolve, or SSH authentication fails.
   */
  connect(options: ConnectOptions): Promise<Connection>;
}

interface KeyPair {
  /** authorized_keys form: `ssh-ed25519 AAAA... flyctl-wasm` */
  publicKey: string;
  /** OpenSSH private key PEM (unencrypted) */
  privateKey: string;
}

interface ConnectOptions {
  /** Token gateway hostname, host:port, or a full ws(s):// URL. Default: gateway.machines.dev */
  gateway?: string;
  /**
   * The macaroon (`FlyV1 fm2_...`) that authenticates the tunnel, or a
   * function returning one. The function is called on every (re)connection,
   * including token refreshes before the gateway's session expiry, so it
   * should return a current token. Throwing or returning a non-string ends
   * the tunnel.
   */
  token: string | (() => string | Promise<string>);
  /** The organization's real slug. "personal" is an API alias the gateway does not know. */
  orgSlug: string;
  /** Custom private network name; omit for the org's default network. */
  network?: string;
  /** Machine address: a 6PN IPv6 literal, or a name resolved through the tunnel's DNS such as `<machine-id>.vm.<app>.internal`. */
  host: string;
  /** SSH port. Default 22. */
  port?: number;
  /** SSH user. Default "root". */
  user?: string;
  /** SSH certificate in authorized_keys form (`ssh-ed25519-cert-v01@openssh.com AAAA...`), issued for `privateKey`. */
  certificate: string;
  /** OpenSSH private key PEM matching `certificate`. */
  privateKey: string;
  /** Run sessions in this container of the machine, as `fly ssh console --container` does. */
  container?: string;
  /** Run sessions on the machine itself rather than a container, as `fly ssh console --machine` does. Mutually exclusive with `container`. */
  machine?: boolean;
  /** Called once when the tunnel ends. `reason` is null after close(), else why the gateway rejected the tunnel for good. */
  onClose?(reason: string | null): void;
}

/** An SSH connection over a WireGuard tunnel. Shells and SFTP sessions are channels on it. */
interface Connection {
  /** The WireGuard peer address the gateway allocated to this tunnel. */
  readonly peerIP: string;
  /** The machine address SSH connected to, after DNS resolution. */
  readonly address: string;

  /**
   * Starts an interactive shell, or runs `options.command`, on the machine.
   * Returns synchronously; output and exit arrive through the callbacks.
   */
  shell(options: ShellOptions): Session;

  /** Opens an SFTP session on the connection. */
  sftp(): Promise<SftpClient>;

  /** Closes every session, the SSH connection and the tunnel. */
  close(): Promise<void>;
}

interface ShellOptions {
  /** Initial terminal width in columns. Default 80. */
  cols?: number;
  /** Initial terminal height in rows. Default 40. */
  rows?: number;
  /** Value for the TERM environment variable. Default "xterm-256color". */
  term?: string;
  /**
   * Command to run instead of a login shell. The machine's SSH server
   * executes it directly, without a shell, exactly as `fly ssh console -C`
   * does: wrap shell syntax in `sh -c '...'`.
   */
  command?: string;
  /** Receives terminal output. Each call is one write from the remote side. */
  onData(data: Uint8Array): void;
  /** Called once when the session ends; `error` is null on a clean exit, else the failure (including a non-zero exit status). */
  onExit?(error: string | null): void;
}

/** A running shell or command. */
interface Session {
  /** Sends terminal input. A string is sent as UTF-8. */
  write(data: Uint8Array | ArrayBuffer | string): void;
  /** Tells the remote pty the terminal was resized. */
  resize(cols: number, rows: number): void;
  /** Ends the session; the remote process may keep running. */
  close(): void;
}

/** A directory entry or stat result. */
interface FileEntry {
  /** Base name for list() results; the path that was asked about for stat(). */
  name: string;
  /** Size in bytes. */
  size: number;
  /** Permission bits, e.g. 0o644. */
  mode: number;
  /** Mode as `ls -l` shows it, e.g. "-rw-r--r--". */
  modeText: string;
  /** Modification time in milliseconds since the epoch. */
  mtime: number;
  isDir: boolean;
  isSymlink: boolean;
}

interface RemoveOptions {
  /** Remove a directory and everything under it. Without it, removing a non-empty directory fails. */
  recursive?: boolean;
}

interface WriteOptions {
  /** Permission bits applied when the writer is closed. Default 0o644. */
  mode?: number;
  /** Replace an existing file. Without it, creation is exclusive and an existing file is an error. */
  overwrite?: boolean;
}

/** An SFTP session. Paths are absolute or relative to the remote user's home. */
interface SftpClient {
  /** Lists a directory. */
  list(path: string): Promise<FileEntry[]>;
  /** Stats a path without following a final symlink. */
  stat(path: string): Promise<FileEntry>;
  /** Canonicalizes a path on the remote side. */
  realpath(path: string): Promise<string>;
  /** Creates a directory and any missing parents. */
  mkdir(path: string): Promise<void>;
  /** Renames a file or directory, replacing the destination if it exists. */
  rename(from: string, to: string): Promise<void>;
  /** Removes a file or directory. */
  remove(path: string, options?: RemoveOptions): Promise<void>;
  /** Sets permission bits. */
  chmod(path: string, mode: number): Promise<void>;
  /** Reads a whole file into memory. Keep this to small files; use the streaming form for anything large. */
  readFile(path: string): Promise<Uint8Array>;
  /**
   * Streams a file in 32 KiB chunks through `onChunk`, waiting for a returned
   * Promise before reading the next one. Resolves with the total byte count.
   */
  readFile(path: string, onChunk: (chunk: Uint8Array) => void | Promise<void>): Promise<number>;
  /** Creates a file for writing. Data goes through the returned writer. */
  writeFile(path: string, options?: WriteOptions): Promise<FileWriter>;
  /** Ends the SFTP session. The connection stays open. */
  close(): Promise<void>;
}

/** An open remote file being written. Calls must be sequential. */
interface FileWriter {
  /** Appends data; resolves with the number of bytes written. */
  write(data: Uint8Array | ArrayBuffer | string): Promise<number>;
  /** Flushes, closes, and applies the mode from WriteOptions. */
  close(): Promise<void>;
}
