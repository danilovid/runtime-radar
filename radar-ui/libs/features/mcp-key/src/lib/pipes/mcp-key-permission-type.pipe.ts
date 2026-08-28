import { Pipe, PipeTransform } from '@angular/core';

import { I18nService } from '@cs/i18n';
import { McpKeyPermissionName } from '@cs/domains/mcp-key';
import { PermissionType } from '@cs/domains/role';

@Pipe({
    name: 'mcpKeyPermissionType',
    pure: false
})
export class McpKeyFeaturePermissionTypePipe implements PipeTransform {
    constructor(private readonly i18nService: I18nService) {}

    transform(type?: PermissionType, permissionName?: McpKeyPermissionName): string {
        switch (type) {
            case PermissionType.CREATE:
                return this.i18nService.translate('McpKey.CreateForm.RulePermissions.Label.CanCreate');
            case PermissionType.READ:
                if (permissionName === McpKeyPermissionName.EVENTS) {
                    return this.i18nService.translate('McpKey.CreateForm.EventPermissions.Label.CanRead');
                }

                return this.i18nService.translate('McpKey.CreateForm.RulePermissions.Label.CanRead');
            case PermissionType.UPDATE:
                return this.i18nService.translate('McpKey.CreateForm.RulePermissions.Label.CanUpdate');
            case PermissionType.DELETE:
                return this.i18nService.translate('McpKey.CreateForm.RulePermissions.Label.CanDelete');
            default:
                return '—';
        }
    }
}
