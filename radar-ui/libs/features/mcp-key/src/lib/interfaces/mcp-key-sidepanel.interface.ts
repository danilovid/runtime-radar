import { PermissionType } from '@cs/domains/role';

export type McpKeySidepanelPermissionMap = {
    [key in string]: Map<PermissionType, boolean>;
};

export interface McpKeySidepanelFormProps {
    permissions: McpKeySidepanelPermissionMap;
}
