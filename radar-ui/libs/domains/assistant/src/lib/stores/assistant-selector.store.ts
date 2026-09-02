import { ActionReducerMap, createFeatureSelector, createSelector } from '@ngrx/store';

import { assistantReducer } from './assistant-reducer.store';
import { AssistantConversation, AssistantMessage, AssistantState } from '../interfaces';

export const ASSISTANT_DOMAIN_KEY = 'assistant';

export interface AssistantDomainState {
    readonly domain: AssistantState;
}

const selectAssistantDomainState = createFeatureSelector<AssistantDomainState>(ASSISTANT_DOMAIN_KEY);
const selectAssistantState = createSelector(selectAssistantDomainState, (state: AssistantDomainState) => state.domain);

export const getAssistantIsOpen = createSelector(selectAssistantState, (state: AssistantState) => state.isOpen);

export const getAssistantView = createSelector(selectAssistantState, (state: AssistantState) => state.view);

export const getAssistantIntegrationId = createSelector(
    selectAssistantState,
    (state: AssistantState) => state.integrationId
);

export const getAssistantSuggestions = createSelector(
    selectAssistantState,
    (state: AssistantState) => state.suggestions
);

export const getAssistantConversations = createSelector(
    selectAssistantState,
    (state: AssistantState) => state.conversations
);

export const getAssistantActiveConversationId = createSelector(
    selectAssistantState,
    (state: AssistantState) => state.activeConversationId
);

export const getAssistantActiveConversation = createSelector(
    getAssistantConversations,
    getAssistantActiveConversationId,
    (conversations: AssistantConversation[], activeId: string) =>
        conversations.find((conversation) => conversation.id === activeId)
);

export const getAssistantMessages = createSelector(
    getAssistantActiveConversation,
    (conversation: AssistantConversation | undefined): AssistantMessage[] => conversation?.messages ?? []
);

export const getAssistantEventId = createSelector(
    getAssistantActiveConversation,
    (conversation: AssistantConversation | undefined) => conversation?.eventId ?? ''
);

export const getAssistantIsStreaming = createSelector(
    selectAssistantState,
    (state: AssistantState) => state.isStreaming
);

export const getAssistantPendingAttachments = createSelector(
    selectAssistantState,
    (state: AssistantState) => state.pendingAttachments
);

export const assistantDomainReducer: ActionReducerMap<AssistantDomainState> = {
    domain: assistantReducer
};
