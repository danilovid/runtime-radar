import { DateAdapter } from '@koobiq/components/core';
import { DateTime } from 'luxon';
import { Router } from '@angular/router';
import { Store } from '@ngrx/store';
import { inject } from '@angular/core';
import { Observable, filter, map, switchMap, take, tap } from 'rxjs';

import { LoadStatus, POLLING_INTERVAL, RouterName } from '@cs/core';

import { McpKeyState } from '../interfaces';
import {
    LOAD_MCP_KEY_ENTITIES_TODO_ACTION,
    POLLING_LOAD_MCP_KEY_ENTITIES_TODO_ACTION
} from '../stores/mcp-key-action.store';
import { getMcpKeyLastUpdate, getMcpKeyLoadStatus } from '../stores/mcp-key-selector.store';

const mcpKeyActivate = (): Observable<boolean> => {
    const dateAdapter = inject<DateAdapter<DateTime>>(DateAdapter);
    const router = inject(Router);
    const store = inject<Store<McpKeyState>>(Store);
    const pollingInterval = inject(POLLING_INTERVAL);

    return store.select(getMcpKeyLoadStatus).pipe(
        tap((status) => {
            if (status === LoadStatus.INIT) {
                store.dispatch(LOAD_MCP_KEY_ENTITIES_TODO_ACTION());
            }
        }),
        filter((status) => [LoadStatus.LOADED, LoadStatus.ERROR].includes(status)),
        map((status) => status === LoadStatus.LOADED),
        tap((isLoaded) => {
            if (!isLoaded) {
                router.navigate([RouterName.ERROR]);
            }
        }),
        switchMap((isLoaded) =>
            store.select(getMcpKeyLastUpdate).pipe(
                take(1),
                tap((lastUpdate) => {
                    const nextUpdate = dateAdapter.today().toMillis() - pollingInterval;
                    if (lastUpdate && nextUpdate > lastUpdate) {
                        store.dispatch(POLLING_LOAD_MCP_KEY_ENTITIES_TODO_ACTION());
                    }
                }),
                map(() => isLoaded)
            )
        )
    );
};

export const mcpKeyActivateGuard = () => mcpKeyActivate();

export const mcpKeyActivateChildGuard = () => mcpKeyActivate();
