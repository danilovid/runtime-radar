import { RolePermission } from '@cs/domains/role';

// should be the same names like PermissionName
export enum McpKeyPermissionName {
    RULES = 'rules',
    EVENTS = 'events',
    SYSTEM_SETTINGS = 'system_settings'
}

export type McpKeyPermissions = {
    [key in McpKeyPermissionName]: RolePermission;
};

/**
 * McpKeyScope is one half of the product a key may reach. Unlike a permission,
 * which the product's roles define and every service enforces, a scope belongs
 * to the key alone and is checked by MCP Server. A key with no scopes reaches
 * both halves.
 */
export enum McpKeyScope {
    RUNTIME_MONITOR = 'runtime_monitor',
    ADMISSION = 'admission'
}

export interface McpKey {
    id: string;
    name: string;
    permissions: McpKeyPermissions;
    scopes?: McpKeyScope[];
    expires_at: string | null; // RFC3339
    invalidated_at?: string; // RFC3339
    /** access_token is the secret itself. It is returned once, when the key is created. */
    access_token?: string;
}
