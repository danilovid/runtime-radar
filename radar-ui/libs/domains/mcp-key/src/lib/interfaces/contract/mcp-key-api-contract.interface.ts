import { McpKey, McpKeyPermissions, McpKeyScope } from './mcp-key-contract.interface';

export interface GetMcpKeysResponse {
    access_tokens: McpKey[];
    total: number;
}

export interface CreateMcpKeyRequest {
    name: string;
    user_id: string;
    permissions: McpKeyPermissions;
    scopes: McpKeyScope[];
    expires_at: string | null; // RFC3339
}

export interface CreateMcpKeyResponse {
    id: string;
    access_token: string;
}

export type EmptyMcpKeyResponse = Record<string, unknown>;
