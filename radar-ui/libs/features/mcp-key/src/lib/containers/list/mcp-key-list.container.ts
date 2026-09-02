import { ActivatedRoute } from '@angular/router';
import { DateTime } from 'luxon';
import { ChangeDetectionStrategy, Component } from '@angular/core';
import { KbqSidepanelConfig, KbqSidepanelPosition, KbqSidepanelService } from '@koobiq/components/sidepanel';
import { Observable, filter, map, switchMap, take } from 'rxjs';

import { ApiPathService } from '@cs/api';
import { AuthStoreService } from '@cs/domains/auth';
import { I18nService } from '@cs/i18n';
import { SharedModalService } from '@cs/shared';
import { ClusterStoreService, RegisteredCluster } from '@cs/domains/cluster';
import {
    CreateMcpKeyRequest,
    McpKey,
    McpKeyPermissionName,
    McpKeyScope,
    McpKeyStoreService
} from '@cs/domains/mcp-key';
import { LoadStatus, CoreUtilsService as utils } from '@cs/core';
import { PermissionName, PermissionType, RolePermissionMap } from '@cs/domains/role';

import { McpKeyFeatureSidepanelFormComponent } from '../../components/sidepanel-form/mcp-key-sidepanel-form.component';
import { McpKeySidepanelFormProps } from '../../interfaces/mcp-key-sidepanel.interface';
import { McpKeyForm, McpKeyPermissionForm, McpKeyPermissionType } from '../../interfaces/mcp-key-form.interface';

@Component({
    templateUrl: './mcp-key-list.container.html',
    styleUrl: './mcp-key-list.container.scss',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class McpKeyFeatureListContainer {
    readonly mcpKeys$: Observable<McpKey[]> = this.mcpKeyStoreService.mcpKeys$.pipe(
        map((mcpKeys) => mcpKeys.sort((a, b) => (a.access_token === b.access_token ? 0 : a.access_token ? -1 : 1)))
    );

    readonly loadStatus$: Observable<LoadStatus> = this.mcpKeyStoreService.loadStatus$;

    readonly clusters$: Observable<RegisteredCluster[]> = this.clusterStoreService.registeredClusters$;

    readonly activeClusterHost$ = this.apiPathService.host$;

    /* eslint @typescript-eslint/dot-notation: "off" */
    readonly permissions: RolePermissionMap = this.route.snapshot.data['permissions'];

    readonly permissionType = PermissionType;

    readonly permissionName = PermissionName;

    readonly loadStatus = LoadStatus;

    readonly mcpKeyPermissionName = McpKeyPermissionName;

    readonly mcpKeyScope = McpKeyScope;

    readonly dateTimeFullFormat = DateTime.DATETIME_FULL;

    constructor(
        private readonly authStoreService: AuthStoreService,
        private readonly apiPathService: ApiPathService,
        private readonly clusterStoreService: ClusterStoreService,
        private readonly i18nService: I18nService,
        private readonly route: ActivatedRoute,
        private readonly sharedModalService: SharedModalService,
        private readonly sidepanelService: KbqSidepanelService,
        private readonly mcpKeyStoreService: McpKeyStoreService
    ) {}

    openCreateSidepanel() {
        const config: KbqSidepanelConfig<McpKeySidepanelFormProps> = {
            position: KbqSidepanelPosition.Right,
            hasBackdrop: true,
            data: {
                permissions: this.permissions
            }
        };

        this.sidepanelService
            .open(McpKeyFeatureSidepanelFormComponent, config)
            .afterClosed()
            .pipe(
                take(1),
                filter(utils.isDefined),
                switchMap((form: McpKeyForm) =>
                    this.authStoreService.credentials$.pipe(
                        take(1),
                        map((credentials) => ({
                            userId: credentials.userId,
                            form
                        }))
                    )
                )
            )
            .subscribe(({ form, userId }) => {
                const request: CreateMcpKeyRequest = {
                    name: form.name,
                    expires_at: form.date ? form.date.toJSDate().toISOString() : null, // RFC3339
                    user_id: userId,
                    permissions: {
                        [McpKeyPermissionName.RULES]: {
                            actions: this.getPermissionActions(form.permissions[McpKeyPermissionName.RULES]),
                            description: ''
                        },
                        [McpKeyPermissionName.EVENTS]: {
                            actions: this.getPermissionActions(form.permissions[McpKeyPermissionName.EVENTS]),
                            description: ''
                        },
                        [McpKeyPermissionName.SYSTEM_SETTINGS]: {
                            actions: this.getPermissionActions(form.permissions[McpKeyPermissionName.SYSTEM_SETTINGS]),
                            description: ''
                        }
                    },
                    scopes: form.scopes
                };

                this.mcpKeyStoreService.createMcpKey(request);
            });
    }

    openDeleteModal(id: string) {
        this.sharedModalService.delete({
            title: this.i18nService.translate('McpKey.DeleteModal.Content.Title'),
            content: this.i18nService.translate('McpKey.DeleteModal.Content.Text'),
            confirmText: this.i18nService.translate('McpKey.DeleteModal.Button.Confirm'),
            cancelText: this.i18nService.translate('McpKey.DeleteModal.Button.Cancel'),
            confirmHandler: () => {
                this.mcpKeyStoreService.deleteMcpKey(id);
            }
        });
    }

    switchCluster(id: string) {
        this.clusterStoreService.switchCluster(id);
    }

    private getPermissionActions(form: McpKeyPermissionForm): PermissionType[] {
        return Object.entries(form).reduce((acc, [key, value]) => {
            if (!value) {
                return acc;
            }

            switch (key) {
                case McpKeyPermissionType.CREATE:
                    acc.push(PermissionType.CREATE);
                    break;
                case McpKeyPermissionType.READ:
                    acc.push(PermissionType.READ);
                    break;
                case McpKeyPermissionType.UPDATE:
                    acc.push(PermissionType.UPDATE);
                    break;
                case McpKeyPermissionType.DELETE:
                    acc.push(PermissionType.DELETE);
                    break;
            }

            return acc;
        }, [] as PermissionType[]);
    }
}
