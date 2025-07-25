import { trim } from 'lodash';
import { useCallback, useEffect, useMemo } from 'react';
import * as React from 'react';

import { CoreApp, isValidGrafanaDuration, LogSortOrderChangeEvent, LogsSortOrder, store } from '@grafana/data';
import { EditorField, EditorRow, QueryOptionGroup } from '@grafana/plugin-ui';
import { getAppEvents } from '@grafana/runtime';
import { AutoSizeInput, RadioButtonGroup } from '@grafana/ui';

import {
  getQueryDirectionLabel,
  preprocessMaxLines,
  queryDirections,
  queryTypeOptions,
} from '../../components/LokiOptionFields';
import { placeHolderScopedVars } from '../../components/monaco-query-field/monaco-completion-provider/validation';
import { LokiDatasource } from '../../datasource';
import { getLokiQueryType, isLogsQuery } from '../../queryUtils';
import { LokiQuery, LokiQueryDirection, LokiQueryType, QueryStats } from '../../types';

export interface Props {
  query: LokiQuery;
  onChange: (update: LokiQuery) => void;
  onRunQuery: () => void;
  app?: CoreApp;
  queryStats: QueryStats | null;
  datasource: LokiDatasource;
}

export const LokiQueryBuilderOptions = React.memo<Props>(
  ({ app, query, onChange, onRunQuery, queryStats, datasource }) => {
    const maxLines = datasource.maxLines;

    useEffect(() => {
      if (app !== CoreApp.Explore && app !== CoreApp.Dashboard && app !== CoreApp.PanelEditor) {
        return;
      }
      // Initialize the query direction according to the current environment.
      if (!query.direction) {
        onChange({ ...query, direction: getDefaultQueryDirection(app) });
      }
    }, [app, onChange, query]);

    useEffect(() => {
      if (query.step && !isValidGrafanaDuration(`${query.step}`) && parseInt(query.step, 10)) {
        onChange({
          ...query,
          step: `${parseInt(query.step, 10)}s`,
        });
      }
    }, [onChange, query]);

    const onQueryTypeChange = (value: LokiQueryType) => {
      onChange({ ...query, queryType: value });
      onRunQuery();
    };

    const onQueryDirectionChange = useCallback(
      (value: LokiQueryDirection) => {
        onChange({ ...query, direction: value });
        onRunQuery();
      },
      [onChange, onRunQuery, query]
    );

    const onLegendFormatChanged = (evt: React.FormEvent<HTMLInputElement>) => {
      onChange({ ...query, legendFormat: evt.currentTarget.value });
      onRunQuery();
    };

    function onMaxLinesChange(e: React.SyntheticEvent<HTMLInputElement>) {
      const newMaxLines = preprocessMaxLines(e.currentTarget.value);
      if (query.maxLines !== newMaxLines) {
        onChange({ ...query, maxLines: newMaxLines });
        onRunQuery();
      }
    }

    function onStepChange(e: React.SyntheticEvent<HTMLInputElement>) {
      onChange({ ...query, step: trim(e.currentTarget.value) });
      onRunQuery();
    }

    useEffect(() => {
      if (app !== CoreApp.Dashboard && app !== CoreApp.PanelEditor) {
        return;
      }
      const subscription = getAppEvents().subscribe(LogSortOrderChangeEvent, (sortEvent: LogSortOrderChangeEvent) => {
        if (query.direction === LokiQueryDirection.Scan) {
          return;
        }
        const newDirection =
          sortEvent.payload.order === LogsSortOrder.Ascending
            ? LokiQueryDirection.Forward
            : LokiQueryDirection.Backward;
        if (newDirection !== query.direction) {
          onQueryDirectionChange(newDirection);
        }
      });
      return () => {
        subscription.unsubscribe();
      };
    }, [app, onQueryDirectionChange, query.direction]);

    let queryType = getLokiQueryType(query);
    const interpolatedQueries = datasource.interpolateVariablesInQueries([query], placeHolderScopedVars);
    const isLogQuery = isLogsQuery(interpolatedQueries[0]?.expr ?? '');
    const filteredQueryTypeOptions = isLogQuery
      ? queryTypeOptions.filter((o) => o.value !== LokiQueryType.Instant)
      : queryTypeOptions;

    // if the state's queryType is still Instant, trigger a change to range for log queries
    if (isLogQuery && queryType === LokiQueryType.Instant) {
      onChange({ ...query, queryType: LokiQueryType.Range });
      queryType = LokiQueryType.Range;
    }

    const isValidStep = useMemo(() => {
      if (!query.step) {
        return true;
      }

      if (typeof query.step === 'string') {
        // If we use a variable as step, we consider it valid
        if (datasource.getVariables().includes(query.step)) {
          return true;
        }
        // Check if the step is a valid Grafana duration
        return isValidGrafanaDuration(query.step) && !isNaN(parseInt(query.step, 10));
      }

      return false;
    }, [query.step, datasource]);

    return (
      <EditorRow>
        <QueryOptionGroup
          title="Options"
          collapsedInfo={getCollapsedInfo(query, queryType, maxLines, isLogQuery, isValidStep, query.direction)}
          queryStats={queryStats}
        >
          <EditorField
            label="Legend"
            tooltip="Series name override or template. Ex. {{hostname}} will be replaced with label value for hostname."
          >
            <AutoSizeInput
              placeholder="{{label}}"
              type="string"
              minWidth={14}
              defaultValue={query.legendFormat}
              onCommitChange={onLegendFormatChanged}
            />
          </EditorField>
          {filteredQueryTypeOptions.length > 1 && (
            <EditorField label="Type">
              <RadioButtonGroup options={filteredQueryTypeOptions} value={queryType} onChange={onQueryTypeChange} />
            </EditorField>
          )}
          {isLogQuery && (
            <>
              <EditorField label="Line limit" tooltip="Upper limit for number of log lines returned by query.">
                <AutoSizeInput
                  className="width-4"
                  placeholder={maxLines.toString()}
                  type="number"
                  min={0}
                  defaultValue={query.maxLines?.toString() ?? ''}
                  onCommitChange={onMaxLinesChange}
                />
              </EditorField>
              <EditorField label="Direction" tooltip="Direction to search for logs.">
                <RadioButtonGroup
                  options={queryDirections}
                  value={query.direction ?? getDefaultQueryDirection(app)}
                  onChange={onQueryDirectionChange}
                />
              </EditorField>
            </>
          )}
          {!isLogQuery && (
            <>
              <EditorField
                label="Step"
                tooltip="Use the step parameter when making metric queries to Loki. If not filled, Grafana's calculated interval will be used. Example valid values: 1s, 5m, 10h, 1d."
                invalid={!isValidStep}
                error={'Invalid step. Example valid values: 1s, 5m, 10h, 1d.'}
              >
                <AutoSizeInput
                  className="width-6"
                  placeholder={'auto'}
                  type="string"
                  value={query.step ?? ''}
                  onCommitChange={onStepChange}
                />
              </EditorField>
            </>
          )}
        </QueryOptionGroup>
      </EditorRow>
    );
  }
);

function getCollapsedInfo(
  query: LokiQuery,
  queryType: LokiQueryType,
  maxLines: number,
  isLogQuery: boolean,
  isValidStep: boolean,
  direction: LokiQueryDirection | undefined
): string[] {
  const queryTypeLabel = queryTypeOptions.find((x) => x.value === queryType);

  const items: string[] = [];

  if (query.legendFormat) {
    items.push(`Legend: ${query.legendFormat}`);
  }

  items.push(`Type: ${queryTypeLabel?.label}`);

  if (isLogQuery && direction) {
    items.push(`Line limit: ${query.maxLines ?? maxLines}`);
    items.push(`Direction: ${getQueryDirectionLabel(direction)}`);
  } else {
    if (query.step) {
      items.push(`Step: ${isValidStep ? query.step : 'Invalid value'}`);
    }
  }

  return items;
}

function getDefaultQueryDirection(app?: CoreApp) {
  if (app !== CoreApp.Explore) {
    /**
     * The default direction is backward because the default sort order is Descending.
     * See:
     * - public/app/features/explore/Logs/Logs.tsx
     * - public/app/plugins/panel/logs/module.tsx
     */
    return LokiQueryDirection.Backward;
  }
  // See app/features/explore/Logs/utils/logs
  const key = 'grafana.explore.logs.sortOrder';
  const storedOrder = store.get(key) || LogsSortOrder.Descending;
  return storedOrder === LogsSortOrder.Ascending ? LokiQueryDirection.Forward : LokiQueryDirection.Backward;
}

LokiQueryBuilderOptions.displayName = 'LokiQueryBuilderOptions';
