import { EntityState } from '@ngrx/entity';

import { LoadStatus } from '@cs/core';

import { McpKey } from '../contract/mcp-key-contract.interface';

export type McpKeyEntityState = EntityState<McpKey>;

export interface McpKeyState {
    loadStatus: LoadStatus;
    lastUpdate: number;
    list: McpKeyEntityState;
}
