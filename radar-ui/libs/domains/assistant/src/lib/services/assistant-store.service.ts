import { Injectable } from '@angular/core';
import { Observable } from 'rxjs';
import { Store } from '@ngrx/store';

import { AssistantMessage, AssistantState } from '../interfaces';
import {
    CLEAR_ASSISTANT_CONVERSATION_TODO_ACTION,
    CLOSE_ASSISTANT_TODO_ACTION,
    OPEN_ASSISTANT_TODO_ACTION,
    SELECT_ASSISTANT_INTEGRATION_TODO_ACTION,
    SEND_ASSISTANT_MESSAGE_TODO_ACTION
} from '../stores/assistant-action.store';
import {
    getAssistantEventId,
    getAssistantIntegrationId,
    getAssistantIsOpen,
    getAssistantIsStreaming,
    getAssistantMessages
} from '../stores/assistant-selector.store';

@Injectable({
    providedIn: 'root'
})
export class AssistantStoreService {
    readonly isOpen$: Observable<boolean> = this.store.select(getAssistantIsOpen);

    readonly isStreaming$: Observable<boolean> = this.store.select(getAssistantIsStreaming);

    readonly integrationId$: Observable<string> = this.store.select(getAssistantIntegrationId);

    readonly messages$: Observable<AssistantMessage[]> = this.store.select(getAssistantMessages);

    readonly eventId$: Observable<string> = this.store.select(getAssistantEventId);

    constructor(private readonly store: Store<AssistantState>) {}

    /**
     * Opens the widget. An event identifier scopes the conversation to that
     * event, and a question is asked straight away.
     */
    open(eventId?: string, question?: string) {
        this.store.dispatch(OPEN_ASSISTANT_TODO_ACTION({ eventId, question }));
    }

    close() {
        this.store.dispatch(CLOSE_ASSISTANT_TODO_ACTION());
    }

    clear() {
        this.store.dispatch(CLEAR_ASSISTANT_CONVERSATION_TODO_ACTION());
    }

    selectIntegration(integrationId: string) {
        this.store.dispatch(SELECT_ASSISTANT_INTEGRATION_TODO_ACTION({ integrationId }));
    }

    send(content: string) {
        this.store.dispatch(SEND_ASSISTANT_MESSAGE_TODO_ACTION({ content }));
    }
}
