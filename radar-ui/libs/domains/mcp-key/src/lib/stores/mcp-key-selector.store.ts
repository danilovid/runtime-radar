import { ActionReducerMap, createFeatureSelector, createSelector } from '@ngrx/store';

import { McpKeyEntityState, McpKeyState } from '../interfaces';
import { mcpKeyEntitySelector, mcpKeyReducer } from './mcp-key-reducer.store';

export const MCP_KEY_DOMAIN_KEY = 'mcp-key';

export interface McpKeyDomainState {
    readonly domain: McpKeyState;
}

const selectMcpKeyDomainState = createFeatureSelector<McpKeyDomainState>(MCP_KEY_DOMAIN_KEY);
const selectMcpKeyState = createSelector(selectMcpKeyDomainState, (state: McpKeyDomainState) => state.domain);
const selectMcpKeyEntityState = createSelector(selectMcpKeyState, (state: McpKeyState) => state.list);

export const getMcpKeyLoadStatus = createSelector(selectMcpKeyState, (state: McpKeyState) => state.loadStatus);

export const getMcpKeyLastUpdate = createSelector(selectMcpKeyState, (state: McpKeyState) => state.lastUpdate);

export const getMcpKeys = createSelector(selectMcpKeyEntityState, (state: McpKeyEntityState) =>
    mcpKeyEntitySelector.selectAll(state)
);

export const mcpKeyDomainReducer: ActionReducerMap<McpKeyDomainState> = {
    domain: mcpKeyReducer
};
