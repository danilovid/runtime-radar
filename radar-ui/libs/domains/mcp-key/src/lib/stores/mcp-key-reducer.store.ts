import { createEntityAdapter } from '@ngrx/entity';
import { Action, ActionReducer, createReducer, on } from '@ngrx/store';

import { LoadStatus } from '@cs/core';

import {
    DELETE_ALL_MCP_KEY_ENTITIES_DOC_ACTION,
    DELETE_MCP_KEY_ENTITY_DOC_ACTION,
    SET_ALL_MCP_KEY_ENTITIES_DOC_ACTION,
    SET_MCP_KEY_ENTITY_DOC_ACTION,
    UPDATE_MCP_KEY_STATE_DOC_ACTION
} from './mcp-key-action.store';
import { McpKey, McpKeyState } from '../interfaces';

const adapter = createEntityAdapter<McpKey>();

const INITIAL_STATE: McpKeyState = {
    loadStatus: LoadStatus.INIT,
    lastUpdate: 0,
    list: adapter.getInitialState()
};

const reducer: ActionReducer<McpKeyState, Action> = createReducer(
    INITIAL_STATE,
    on(UPDATE_MCP_KEY_STATE_DOC_ACTION, (state, values) => ({ ...state, ...values })),
    on(SET_ALL_MCP_KEY_ENTITIES_DOC_ACTION, (state, { list }) => ({
        ...state,
        list: adapter.setAll(list, state.list)
    })),
    on(DELETE_ALL_MCP_KEY_ENTITIES_DOC_ACTION, (state) => ({
        ...state,
        list: adapter.removeAll(state.list)
    })),
    on(SET_MCP_KEY_ENTITY_DOC_ACTION, (state, { item }) => ({
        ...state,
        list: adapter.setOne(item, state.list)
    })),
    on(DELETE_MCP_KEY_ENTITY_DOC_ACTION, (state, { id }) => ({
        ...state,
        list: adapter.removeOne(id, state.list)
    }))
);

export const mcpKeyEntitySelector = adapter.getSelectors();

export function mcpKeyReducer(state: McpKeyState | undefined, action: Action): McpKeyState {
    return reducer(state, action);
}
