import React, { FC, useState } from 'react';
import { queryCache, useMutation } from 'react-query';
import { css } from 'emotion';
import intl from 'react-intl-universal';

import { createSchedule } from 'api';
import { Button, Legend } from '@grafana/ui';
import { EditReportScheduleModal } from '../Groups/EditReportScheduleModal';
import { Schedule } from 'common/types';
import { ScheduleList } from './ScheduleList';
import { AppRootProps } from '@grafana/data';

interface Props extends AppRootProps {}

const headerAdjustment = css`
  display: flex;
  justify-content: flex-end;
  margin-bottom: 10px;
`;

export const ReportSchedulesTab: FC<Props> = () => {
  const [activeGroup, setActiveSchedule] = useState<Schedule | null>(null);

  const [newSchedule] = useMutation(createSchedule, {
    onSuccess: () => queryCache.refetchQueries('reportSchedules'),
  });

  const onNewSchedule = async () => {
    return newSchedule();
  };

  return (
    <div>
      <div className={headerAdjustment}>
        <Legend>{intl.get('report_schedules')}</Legend>
        <Button onClick={onNewSchedule} variant="primary">
          {intl.get('add_schedule')}
        </Button>
      </div>
      <ScheduleList onRowPress={setActiveSchedule} />
      {activeGroup && (
        <EditReportScheduleModal
          reportSchedule={activeGroup}
          isOpen={!!activeGroup}
          onClose={() => setActiveSchedule(null)}
        />
      )}
    </div>
  );
};
