import { createAction, props } from '@ngrx/store';

import { CreateMcpKeyRequest, McpKey, McpKeyState } from '../interfaces';

export const LOAD_MCP_KEY_ENTITIES_TODO_ACTION = createAction('[McpKey] Load');

export const POLLING_LOAD_MCP_KEY_ENTITIES_TODO_ACTION = createAction('[McpKey] Polling Load');

export const CREATE_MCP_KEY_ENTITY_TODO_ACTION = createAction(
    '[McpKey] Create',
    props<{ item: CreateMcpKeyRequest }>()
);

export const DELETE_MCP_KEY_ENTITY_TODO_ACTION = createAction('[McpKey] Delete', props<{ id: string }>());

export const UPDATE_MCP_KEY_STATE_DOC_ACTION = createAction(
    '[McpKey] (Doc) Update State',
    props<Partial<Omit<McpKeyState, 'list'>>>()
);

export const SET_ALL_MCP_KEY_ENTITIES_DOC_ACTION = createAction('[McpKey] (Doc) Set All', props<{ list: McpKey[] }>());

export const DELETE_ALL_MCP_KEY_ENTITIES_DOC_ACTION = createAction('[McpKey] (Doc) Delete All');

export const SET_MCP_KEY_ENTITY_DOC_ACTION = createAction('[McpKey] (Doc) Set One', props<{ item: McpKey }>());

export const DELETE_MCP_KEY_ENTITY_DOC_ACTION = createAction('[McpKey] (Doc) Delete', props<{ id: string }>());
