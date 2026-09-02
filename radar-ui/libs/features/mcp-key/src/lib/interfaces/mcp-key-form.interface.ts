import { DateTime } from 'luxon';

import { McpKeyPermissionName, McpKeyScope } from '@cs/domains/mcp-key';

export enum McpKeyExpiryDatePreset {
    WEEK = 'WEEK',
    MONTH = 'MONTH',
    QUARTER = 'QUARTER',
    INDEFINITELY = 'INDEFINITELY',
    CUSTOM = 'CUSTOM'
}

export interface McpKeyExpiryDatePresetOption {
    id: McpKeyExpiryDatePreset;
    localizationKey: string;
}

export enum McpKeyPermissionType {
    CREATE = 'canCreate',
    READ = 'canRead',
    UPDATE = 'canUpdate',
    DELETE = 'canDelete'
}

export type McpKeyPermissionForm = {
    [key in McpKeyPermissionType]: boolean;
};

export type McpKeyPermissionRecord = {
    [key in McpKeyPermissionName]: McpKeyPermissionForm;
};

export interface McpKeyForm {
    name: string;
    date: DateTime;
    preset: McpKeyExpiryDatePreset;
    permissions: McpKeyPermissionRecord;
    /** scopes narrow the key to one half of the product. Empty means both. */
    scopes: McpKeyScope[];
}

export interface McpKeyScopeOption {
    id: McpKeyScope;
    localizationKey: string;
    descriptionLocalizationKey: string;
}
