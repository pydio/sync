package control

import (
	"context"
	"fmt"
	"time"

	"github.com/pydio/cells/common/sync/merger"

	"github.com/pydio/sync/config"

	"github.com/pydio/cells/common/log"
	"github.com/pydio/cells/common/service/context"

	"github.com/pkg/errors"

	"github.com/pydio/cells/common/sync/model"
	"github.com/pydio/cells/common/sync/task"
	"github.com/pydio/sync/endpoint"
)

type Syncer struct {
	task   *task.Sync
	ticker *time.Ticker
	stop   chan bool
	uuid   string

	eventsChan  chan interface{}
	batchStatus chan merger.BatchProcessStatus
	batchDone   chan bool
}

func NewSyncer(conf *config.Task) (*Syncer, error) {
	parseMessage := "invalid arguments: please provide left and right endpoints using a valid URI"
	if conf.LeftURI == "" || conf.RightURI == "" {
		return nil, fmt.Errorf(parseMessage)
	}
	leftEndpoint, err := endpoint.EndpointFromURI(conf.LeftURI, conf.RightURI)
	if err != nil {
		return nil, errors.Wrap(err, parseMessage)
	}
	rightEndpoint, err := endpoint.EndpointFromURI(conf.RightURI, conf.LeftURI)
	if err != nil {
		return nil, errors.Wrap(err, parseMessage)
	}

	var dir model.DirectionType
	switch conf.Direction {
	case "Bi":
		dir = model.DirectionBi
	case "Left":
		dir = model.DirectionLeft
	case "Right":
		dir = model.DirectionRight
	default:
		return nil, fmt.Errorf("unsupported direction type, please use one of Bi, Left, Right")
	}
	taskUuid := conf.Uuid
	syncTask := task.NewSync(context.Background(), leftEndpoint, rightEndpoint, dir)
	eventsChan := make(chan interface{})
	batchStatus := make(chan merger.BatchProcessStatus)
	batchDone := make(chan bool)
	syncTask.SetSyncEventsChan(batchStatus, batchDone, eventsChan)
	return &Syncer{
		uuid:        taskUuid,
		task:        syncTask,
		eventsChan:  eventsChan,
		batchStatus: batchStatus,
		batchDone:   batchDone,
		stop:        make(chan bool, 1),
	}, nil

}

func (s *Syncer) Serve() {

	ctx := servicecontext.WithServiceName(context.Background(), "sync-task")
	ctx = servicecontext.WithServiceColor(ctx, servicecontext.ServiceColorGrpc)

	log.Logger(ctx).Info("Starting Sync Service")
	s.task.SetSnapshotFactory(endpoint.NewSnapshotFactory(s.uuid))
	s.task.Start(ctx)
	bus := GetBus()
	topic := bus.Sub(TopicSyncAll, TopicSync_+s.uuid)
	s.ticker = time.NewTicker(10 * time.Minute)

	for {
		select {

		case l := <-s.batchStatus:
			msg := "STATUS: " + l.StatusString
			if l.Progress > 0 {
				msg += fmt.Sprintf(" - Progress: %d%%", int64(l.Progress*100))
			}
			if l.IsError {
				log.Logger(ctx).Error(msg)
			} else {
				log.Logger(ctx).Info(msg)
			}

		case <-s.batchDone:
			log.Logger(ctx).Info("BATCH FINISHED")

		case e := <-s.eventsChan:

			GetBus().Pub(e, TopicSync_+s.uuid)

		case <-s.stop:

			s.task.Shutdown()
			s.ticker.Stop()
			close(s.eventsChan)
			close(s.batchDone)
			close(s.batchStatus)
			log.Logger(ctx).Info("Stopping Service")
			return

		case <-s.ticker.C:

			s.task.Resync(ctx, false, false)

		case message := <-topic:

			switch message {
			case MessageResync:
				s.task.Resync(ctx, false, true)
			case MessageResyncDry:
				s.task.Resync(ctx, true, true)
			case MessageSyncLoop:
				s.task.Resync(ctx, false, false)
			case model.WatchDisconnected:
				log.Logger(ctx).Info("Currently disconnected")
			case model.WatchConnected:
				log.Logger(ctx).Info("Connected, launching a sync loop")
				s.task.Resync(ctx, false, false)
			case MessagePause:
				s.task.Pause()
			case MessageResume:
				s.task.Resume()
				s.task.Resync(ctx, false, false)
			}

		}
	}

}

func (s *Syncer) Stop() {
	s.stop <- true
}