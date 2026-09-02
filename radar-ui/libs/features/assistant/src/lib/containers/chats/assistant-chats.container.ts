import { DateTime } from 'luxon';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { ActivatedRoute, Router } from '@angular/router';
import { ChangeDetectionStrategy, Component, DestroyRef, OnInit } from '@angular/core';
import { Observable, map, take } from 'rxjs';

import { I18nService } from '@cs/i18n';
import { IntegrationStoreService } from '@cs/domains/integration';
import {
    ASSISTANT_QUICK_ACTIONS,
    AssistantConversation,
    AssistantRole,
    AssistantStoreService
} from '@cs/domains/assistant';
import { LoadStatus, RouterName } from '@cs/core';
import { PermissionName, PermissionType, RolePermissionMap } from '@cs/domains/role';

@Component({
    templateUrl: './assistant-chats.container.html',
    styleUrl: './assistant-chats.container.scss',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AssistantFeatureChatsContainer implements OnInit {
    readonly dateTimeShortFormat = DateTime.DATETIME_SHORT;

    readonly conversations$: Observable<AssistantConversation[]> = this.assistantStoreService.conversations$;

    /**
     * Whether anything can answer at all. The assistant speaks through an AI
     * integration, so without one the page has nothing to offer and points at
     * the place where an integration is added instead.
     */
    readonly hasIntegration$: Observable<boolean> = this.integrationStoreService.aiIntegrations$.pipe(
        map((integrations) => !!integrations.length)
    );

    readonly quickActions = ASSISTANT_QUICK_ACTIONS;

    /* eslint @typescript-eslint/dot-notation: "off" */
    private readonly permissions: RolePermissionMap = this.route.snapshot.data['permissions'];

    /** Sending the user to a page their role forbids would only dead-end them. */
    readonly canReadIntegrations = !!this.permissions?.[PermissionName.INTEGRATIONS]?.has(PermissionType.READ);

    constructor(
        private readonly assistantStoreService: AssistantStoreService,
        private readonly destroyRef: DestroyRef,
        private readonly i18nService: I18nService,
        private readonly integrationStoreService: IntegrationStoreService,
        private readonly route: ActivatedRoute,
        private readonly router: Router
    ) {}

    ngOnInit() {
        // The integrations page loads these through its own route guard, and
        // this page can be opened without ever going there.
        if (!this.canReadIntegrations) {
            return;
        }

        this.integrationStoreService.loadStatus$
            .pipe(take(1), takeUntilDestroyed(this.destroyRef))
            .subscribe((status) => {
                if (status === LoadStatus.INIT) {
                    this.integrationStoreService.load();
                }
            });
    }

    startChat() {
        this.assistantStoreService.open();
        this.assistantStoreService.startChat();
    }

    askKey(localizationKey: string) {
        this.assistantStoreService.open();
        this.assistantStoreService.startChat(this.i18nService.translate(localizationKey));
    }

    openConversation(conversationId: string) {
        this.assistantStoreService.openConversation(conversationId);
    }

    deleteConversation(event: Event, conversationId: string) {
        event.stopPropagation();
        this.assistantStoreService.deleteConversation(conversationId);
    }

    openIntegrations() {
        this.router.navigate([RouterName.SETTINGS, RouterName.INTEGRATIONS]);
    }

    /** The last thing said in a conversation, for the list row. */
    snippet(conversation: AssistantConversation): string {
        const last = conversation.messages[conversation.messages.length - 1];

        if (!last) {
            return '';
        }

        const prefix = last.role === AssistantRole.ASSISTANT ? '' : '> ';

        return prefix + last.content.slice(0, 120);
    }
}
