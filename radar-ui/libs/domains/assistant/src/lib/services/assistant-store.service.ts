import { Injectable } from '@angular/core';
import { Observable } from 'rxjs';
import { Store } from '@ngrx/store';

import {
    ATTACH_ASSISTANT_FILES_TODO_ACTION,
    CLOSE_ASSISTANT_TODO_ACTION,
    CONFIRM_ASSISTANT_ACTION_TODO_ACTION,
    DECLINE_ASSISTANT_ACTION_TODO_ACTION,
    DELETE_ASSISTANT_CONVERSATION_TODO_ACTION,
    LOAD_ASSISTANT_CHATS_TODO_ACTION,
    OPEN_ASSISTANT_CONVERSATION_TODO_ACTION,
    OPEN_ASSISTANT_TODO_ACTION,
    REMOVE_ASSISTANT_ATTACHMENT_TODO_ACTION,
    SELECT_ASSISTANT_INTEGRATION_TODO_ACTION,
    SEND_ASSISTANT_MESSAGE_TODO_ACTION,
    SHOW_ASSISTANT_VIEW_TODO_ACTION,
    START_ASSISTANT_CHAT_TODO_ACTION
} from '../stores/assistant-action.store';
import {
    AssistantAttachment,
    AssistantConversation,
    AssistantEventKind,
    AssistantMessage,
    AssistantMode,
    AssistantState,
    AssistantView
} from '../interfaces';
import {
    getAssistantConversations,
    getAssistantEventId,
    getAssistantIntegrationId,
    getAssistantIsOpen,
    getAssistantIsStreaming,
    getAssistantMessages,
    getAssistantPendingAttachments,
    getAssistantSuggestions,
    getAssistantView
} from '../stores/assistant-selector.store';

@Injectable({
    providedIn: 'root'
})
export class AssistantStoreService {
    readonly isOpen$: Observable<boolean> = this.store.select(getAssistantIsOpen);

    readonly isStreaming$: Observable<boolean> = this.store.select(getAssistantIsStreaming);

    readonly view$: Observable<AssistantView> = this.store.select(getAssistantView);

    readonly integrationId$: Observable<string> = this.store.select(getAssistantIntegrationId);

    /** Follow-ups the model proposed for the answer on screen. */
    readonly suggestions$: Observable<string[]> = this.store.select(getAssistantSuggestions);

    readonly conversations$: Observable<AssistantConversation[]> = this.store.select(getAssistantConversations);

    readonly messages$: Observable<AssistantMessage[]> = this.store.select(getAssistantMessages);

    readonly eventId$: Observable<string> = this.store.select(getAssistantEventId);

    readonly pendingAttachments$: Observable<AssistantAttachment[]> = this.store.select(getAssistantPendingAttachments);

    constructor(private readonly store: Store<AssistantState>) {}

    /**
     * Opens the panel. An event identifier or a question starts a conversation
     * straight away; without either the panel shows its home screen. That is
     * what the entry points outside the widget use: "Explain event", the
     * summary on the events page, and "Report a problem".
     */
    /** Reads back the conversations the server kept for this user. */
    loadChats() {
        this.store.dispatch(LOAD_ASSISTANT_CHATS_TODO_ACTION());
    }

    open(options: { eventId?: string; eventKind?: AssistantEventKind; question?: string; mode?: AssistantMode } = {}) {
        this.store.dispatch(OPEN_ASSISTANT_TODO_ACTION(options));
    }

    close() {
        this.store.dispatch(CLOSE_ASSISTANT_TODO_ACTION());
    }

    showView(view: AssistantView) {
        this.store.dispatch(SHOW_ASSISTANT_VIEW_TODO_ACTION({ view }));
    }

    startChat(question?: string, mode?: AssistantMode) {
        this.store.dispatch(START_ASSISTANT_CHAT_TODO_ACTION({ question, mode }));
    }

    /** Approves the change the assistant proposed in that message. */
    confirmAction(messageId: string, actionId: string) {
        this.store.dispatch(CONFIRM_ASSISTANT_ACTION_TODO_ACTION({ messageId, actionId }));
    }

    declineAction(messageId: string) {
        this.store.dispatch(DECLINE_ASSISTANT_ACTION_TODO_ACTION({ messageId }));
    }

    openConversation(conversationId: string) {
        this.store.dispatch(OPEN_ASSISTANT_CONVERSATION_TODO_ACTION({ conversationId }));
    }

    deleteConversation(conversationId: string) {
        this.store.dispatch(DELETE_ASSISTANT_CONVERSATION_TODO_ACTION({ conversationId }));
    }

    selectIntegration(integrationId: string) {
        this.store.dispatch(SELECT_ASSISTANT_INTEGRATION_TODO_ACTION({ integrationId }));
    }

    send(content: string) {
        this.store.dispatch(SEND_ASSISTANT_MESSAGE_TODO_ACTION({ content }));
    }

    attach(files: File[]) {
        this.store.dispatch(ATTACH_ASSISTANT_FILES_TODO_ACTION({ files }));
    }

    removeAttachment(name: string) {
        this.store.dispatch(REMOVE_ASSISTANT_ATTACHMENT_TODO_ACTION({ name }));
    }
}
