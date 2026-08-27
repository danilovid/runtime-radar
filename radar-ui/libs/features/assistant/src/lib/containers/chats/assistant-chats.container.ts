import { Observable } from 'rxjs';
import { ChangeDetectionStrategy, Component } from '@angular/core';

import { I18nService } from '@cs/i18n';
import {
    ASSISTANT_QUICK_ACTIONS,
    AssistantConversation,
    AssistantRole,
    AssistantStoreService
} from '@cs/domains/assistant';

@Component({
    templateUrl: './assistant-chats.container.html',
    styleUrl: './assistant-chats.container.scss',
    changeDetection: ChangeDetectionStrategy.OnPush
})
export class AssistantFeatureChatsContainer {
    readonly conversations$: Observable<AssistantConversation[]> = this.assistantStoreService.conversations$;

    readonly quickActions = ASSISTANT_QUICK_ACTIONS;

    constructor(
        private readonly assistantStoreService: AssistantStoreService,
        private readonly i18nService: I18nService
    ) {}

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
