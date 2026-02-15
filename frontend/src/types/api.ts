export interface Network {
  id: number;
  name: string;
  interface: string;
  mode: 'gateway' | 'site-to-site' | 'hub-routed';
  subnet: string;
  listen_port: number;
  public_key: string;
  dns_servers: string;
  nat_enabled: boolean;
  inter_peer_routing: boolean;
  enabled: boolean;
  created_at: number;
  updated_at: number;
}

export interface CreateNetworkRequest {
  name: string;
  mode: 'gateway' | 'site-to-site' | 'hub-routed';
  subnet: string;
  listen_port: number;
  dns_servers: string;
  nat_enabled: boolean;
  inter_peer_routing: boolean;
}

export type UpdateNetworkRequest = Partial<CreateNetworkRequest>;

export interface Peer {
  id: number;
  network_id: number;
  name: string;
  email: string;
  public_key: string;
  allowed_ips: string;
  endpoint: string;
  persistent_keepalive: number;
  role: 'client' | 'site-gateway';
  site_networks: string;
  enabled: boolean;
  online: boolean;
  last_handshake: number;
  transfer_rx: number;
  transfer_tx: number;
  created_at: number;
  updated_at: number;
}

export interface CreatePeerRequest {
  name: string;
  email?: string;
  role: 'client' | 'site-gateway';
  persistent_keepalive?: number;
}

export type UpdatePeerRequest = Partial<CreatePeerRequest>;

export interface PeerStatus {
  peer_id: number;
  name: string;
  public_key: string;
  online: boolean;
  last_handshake: number;
  transfer_rx: number;
  transfer_tx: number;
}

export interface NetworkStatus {
  id: number;
  name: string;
  interface: string;
  enabled: boolean;
  up: boolean;
  listen_port: number;
  peers: PeerStatus[];
}

export interface StatusResponse {
  networks: NetworkStatus[];
}

export interface AuthLoginRequest {
  username: string;
  password: string;
}

export interface AuthLoginResponse {
  token: string;
  user: AuthUser;
}

export interface AuthUser {
  id: number;
  username: string;
}

export interface ApiError {
  error: {
    code: string;
    message: string;
  };
}

export interface TransferStats {
  timestamp: number;
  transfer_rx: number;
  transfer_tx: number;
}

// ── Bridge types ────────────────────────────────────────────────────

export interface Bridge {
  id: number;
  network_a_id: number;
  network_b_id: number;
  network_a_name: string;
  network_b_name: string;
  interface_a: string;
  interface_b: string;
  direction: 'a_to_b' | 'b_to_a' | 'bidirectional';
  allowed_cidrs: string;
  enabled: boolean;
  created_at: number;
  updated_at: number;
}

export interface CreateBridgeRequest {
  network_a_id: number;
  network_b_id: number;
  direction: 'a_to_b' | 'b_to_a' | 'bidirectional';
  allowed_cidrs?: string;
}

// ── Setup wizard types ──────────────────────────────────────────────

export interface SetupStatusResponse {
  complete: boolean;
  current_step: number;
  wg_interfaces?: string[];
}

export interface SetupStep1Request {
  otp: string;
  username: string;
  password: string;
}

export interface SetupStep1Response {
  user: AuthUser;
}

export interface SetupStep2Request {
  public_ip: string;
  hostname: string;
  dns_servers: string;
}

export interface SetupStep3Request {
  name: string;
  mode: 'gateway' | 'site-to-site' | 'hub-routed';
  subnet: string;
  listen_port: number;
  nat_enabled: boolean;
  inter_peer_routing: boolean;
}

export interface SetupStep3Response {
  network: Network;
}

export interface SetupStep4Request {
  name: string;
  role: 'client' | 'site-gateway';
  tunnel_type: 'full' | 'split';
}

export interface SetupStep4Response {
  peer: Peer;
  config: string;
  qr_data: string;
}

export interface DetectIPResponse {
  public_ip: string;
}
