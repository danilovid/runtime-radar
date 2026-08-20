import { ActionReducerMap, createFeatureSelector, createSelector } from '@ngrx/store';

import { AssistantState } from '../interfaces';
import { assistantReducer } from './assistant-reducer.store';

export const ASSISTANT_DOMAIN_KEY = 'assistant';

export interface AssistantDomainState {
    readonly domain: AssistantState;
}

const selectAssistantDomainState = createFeatureSelector<AssistantDomainState>(ASSISTANT_DOMAIN_KEY);
const selectAssistantState = createSelector(selectAssistantDomainState, (state: AssistantDomainState) => state.domain);

export const getAssistantIsOpen = createSelector(selectAssistantState, (state: AssistantState) => state.isOpen);

export const getAssistantIntegrationId = createSelector(
    selectAssistantState,
    (state: AssistantState) => state.integrationId
);

export const getAssistantMessages = createSelector(selectAssistantState, (state: AssistantState) => state.messages);

export const getAssistantIsStreaming = createSelector(
    selectAssistantState,
    (state: AssistantState) => state.isStreaming
);

export const getAssistantEventId = createSelector(selectAssistantState, (state: AssistantState) => state.eventId);

export const assistantDomainReducer: ActionReducerMap<AssistantDomainState> = {
    domain: assistantReducer
};
