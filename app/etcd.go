package app

import (
	"context"
	"encoding/json"

	"github.com/coreos/etcd/mvcc/mvccpb"

	"github.com/coreos/etcd/clientv3"
	"ojbk.io/gopherCron/common"
	"ojbk.io/gopherCron/errors"
	"ojbk.io/gopherCron/utils"
)

// SaveTask save task to etcd
// return oldtask & error
func (a *app) SaveTask(task *common.TaskInfo) (*common.TaskInfo, error) {
	var (
		saveKey  string
		saveByte []byte
		putResp  *clientv3.PutResponse
		oldTask  *common.TaskInfo
		ctx      context.Context
		errObj   errors.Error
		err      error
	)

	// build etcd save key
	saveKey = common.BuildKey(task.ProjectID, task.TaskID)

	// task to json
	if saveByte, err = json.Marshal(task); err != nil {
		errObj = errors.ErrInternalError
		errObj.Log = "[Etcd - SaveTask] json.mashal task error:" + err.Error()
		return nil, errObj
	}

	ctx, _ = utils.GetContextWithTimeout()
	// save to etcd
	if putResp, err = a.etcd.KV().Put(ctx, saveKey, string(saveByte), clientv3.WithPrevKV()); err != nil {
		errObj = errors.ErrInternalError
		errObj.Log = "[Etcd - SaveTask] etcd client kv put error:" + err.Error()
		return nil, errObj
	}

	// oldtask exist
	if putResp.PrevKv != nil {
		// if oldtask unmarshal error
		// don't care because this err doesn't affect result
		json.Unmarshal([]byte(putResp.PrevKv.Value), &oldTask)
	}

	return oldTask, nil
}

// TemporarySchedulerTask 临时调度任务
func (a *app) TemporarySchedulerTask(task *common.TaskInfo) error {
	var (
		schedulerKey   string
		saveByte       []byte
		leaseGrantResp *clientv3.LeaseGrantResponse
		ctx            context.Context
		errObj         errors.Error
		err            error
	)

	// task to json
	if saveByte, err = json.Marshal(task); err != nil {
		errObj = errors.ErrInternalError
		errObj.Log = "[Etcd - TemporarySchedulerTask] json.mashal task error:" + err.Error()
		return errObj
	}

	// build etcd save key
	schedulerKey = common.BuildSchedulerKey(task.ProjectID, task.TaskID)

	ctx, _ = utils.GetContextWithTimeout()
	// make lease to notify worker
	// 创建一个租约 让其稍后过期并自动删除
	if leaseGrantResp, err = a.etcd.Lease().Grant(ctx, 1); err != nil {
		errObj = errors.ErrInternalError
		errObj.Log = "[Etcd - TemporarySchedulerTask] lease grant error:" + err.Error()
		return errObj
	}

	ctx, _ = utils.GetContextWithTimeout()
	// save to etcd
	if _, err = a.etcd.KV().Put(ctx, schedulerKey, string(saveByte), clientv3.WithLease(leaseGrantResp.ID)); err != nil {
		errObj = errors.ErrInternalError
		errObj.Log = "[Etcd - TemporarySchedulerTask] etcd client kv put error:" + err.Error()
		return errObj
	}

	return nil
}

func (a *app) DeleteTask(projectID int64, taskID string) (*common.TaskInfo, error) {
	var (
		deleteKey string
		delResp   *clientv3.DeleteResponse
		oldTask   *common.TaskInfo
		ctx       context.Context
		errObj    errors.Error
		err       error
	)

	// build etcd delete key
	deleteKey = common.BuildKey(projectID, taskID)

	ctx, _ = utils.GetContextWithTimeout()
	// save to etcd
	if delResp, err = a.etcd.KV().Delete(ctx, deleteKey, clientv3.WithPrevKV(), clientv3.WithPrefix()); err != nil {
		errObj = errors.ErrInternalError
		errObj.Log = "[Etcd - DeleteTask] etcd client kv delete error:" + err.Error()
		return nil, errObj
	}

	if taskID != "" && len(delResp.PrevKvs) != 0 {
		json.Unmarshal([]byte(delResp.PrevKvs[0].Value), &oldTask)
	}

	return oldTask, nil
}

// GetTask 获取任务
func (a *app) GetTask(projectID int64, nameID string) (*common.TaskInfo, error) {
	var (
		saveKey string
		getResp *clientv3.GetResponse
		task    *common.TaskInfo
		ctx     context.Context
		errObj  errors.Error
		err     error
	)

	// build etcd save key
	saveKey = common.BuildKey(projectID, nameID) // 保存的key同样也是获取的key

	ctx, _ = utils.GetContextWithTimeout()

	if getResp, err = a.etcd.KV().Get(ctx, saveKey); err != nil {
		errObj = errors.ErrInternalError
		errObj.Log = "[Etcd - GetTask] etcd client kv get one error:" + err.Error()
		return nil, errObj
	}

	if getResp.Count > 1 {
		errObj = errors.ErrInternalError
		errObj.Log = "[Etcd - GetTask] etcd client kv get one task but result > 1"
		return nil, errObj
	} else if getResp.Count == 0 {
		return nil, errors.ErrDataNotFound
	}

	task = &common.TaskInfo{}
	if err = json.Unmarshal(getResp.Kvs[0].Value, task); err != nil {
		errObj = errors.ErrInternalError
		errObj.Log = "[Etcd - GetTask] task json.Unmarshal error:" + err.Error()
		return nil, errObj
	}

	return task, nil
}

// GetTaskList 获取任务列表
func (a *app) GetTaskList(projectID int64) ([]*common.TaskInfo, error) {
	var (
		preKey   string
		getResp  *clientv3.GetResponse
		taskList []*common.TaskInfo
		task     *common.TaskInfo
		kvPair   *mvccpb.KeyValue
		ctx      context.Context
		errObj   errors.Error
		err      error
	)

	// build etcd pre key
	preKey = common.BuildKey(projectID, "")

	ctx, _ = utils.GetContextWithTimeout()
	if getResp, err = a.etcd.KV().Get(ctx, preKey,
		clientv3.WithPrefix()); err != nil {
		errObj = errors.ErrInternalError
		errObj.Log = "[Etcd - GetTaskList] etcd client kv getlist error:" + err.Error()
		return nil, errObj
	}

	// init array space
	taskList = make([]*common.TaskInfo, 0)

	// range list to unmarshal
	for _, kvPair = range getResp.Kvs {
		task = &common.TaskInfo{}
		if err = json.Unmarshal(kvPair.Value, task); err != nil {
			continue
		}

		taskList = append(taskList, task)
	}

	return taskList, nil
}

// GetTaskList 获取任务列表
func (a *app) GetProjectTaskCount(projectID int64) (int64, error) {
	var (
		preKey  string
		getResp *clientv3.GetResponse
		errObj  errors.Error
		err     error
		ctx     context.Context
	)

	// build etcd pre key
	preKey = common.BuildKey(projectID, "")
	ctx, _ = utils.GetContextWithTimeout()
	if getResp, err = a.etcd.KV().Get(ctx, preKey,
		clientv3.WithPrefix(), clientv3.WithCountOnly()); err != nil {
		errObj = errors.ErrInternalError
		errObj.Log = "[Etcd - GetProjectTaskCount] etcd client kv error:" + err.Error()
		return 0, errObj
	}

	return getResp.Count, nil
}

// KillTask 强行结束任务
func (a *app) KillTask(projectID int64, name string) error {
	var (
		killKey        string
		leaseGrantResp *clientv3.LeaseGrantResponse
		errObj         errors.Error
		err            error
		ctx            context.Context
	)

	killKey = common.BuildKillKey(projectID, name)
	ctx, _ = utils.GetContextWithTimeout()
	// make lease to notify worker
	// 创建一个租约 让其稍后过期并自动删除
	if leaseGrantResp, err = a.etcd.Lease().Grant(ctx, 1); err != nil {
		errObj = errors.ErrInternalError
		errObj.Log = "[Etcd - KillTask] lease grant error:" + err.Error()
		return errObj
	}

	ctx, _ = utils.GetContextWithTimeout()
	if _, err = a.etcd.KV().Put(ctx, killKey, "", clientv3.WithLease(leaseGrantResp.ID)); err != nil {
		errObj = errors.ErrInternalError
		errObj.Log = "[Etcd - KillTask] put kill task error:" + err.Error()
		return errObj
	}

	return nil
}

// GetWorkerList 获取节点列表
func (a *app) GetWorkerList(projectID int64) ([]string, error) {
	var (
		preKey  string
		err     error
		errObj  errors.Error
		getResp *clientv3.GetResponse
		kv      *mvccpb.KeyValue
		ctx     context.Context
		res     []string
	)

	preKey = common.BuildRegisterKey(projectID, "")
	ctx, _ = utils.GetContextWithTimeout()
	if getResp, err = a.etcd.KV().Get(ctx, preKey, clientv3.WithPrefix()); err != nil {
		errObj = errors.ErrInternalError
		errObj.Log = "[Etcd - GetWorkerList] get preKey error:" + err.Error()
		return nil, errObj
	}

	for _, kv = range getResp.Kvs {
		res = append(res, common.ExtractWorkerIP(projectID, string(kv.Key)))
	}

	return res, nil
}

func (a *app) DeleteAll() error {
	var (
		deleteKey string
		ctx       context.Context
		errObj    errors.Error
		err       error
	)

	// build etcd delete key
	deleteKey = common.ETCD_PREFIX + "/"
	ctx, _ = utils.GetContextWithTimeout()
	// save to etcd
	if _, err = a.etcd.KV().Delete(ctx, deleteKey, clientv3.WithPrevKV(), clientv3.WithPrefix()); err != nil {
		errObj = errors.ErrInternalError
		errObj.Log = "[Etcd - DeleteAll] etcd client kv delete error:" + err.Error()
		return errObj
	}

	return nil
}

// SaveMonitor 保存节点的监控信息
func (a *app) SaveMonitor(ip string, monitorInfo []byte) error {
	var (
		monitorKey     string
		leaseGrantResp *clientv3.LeaseGrantResponse
		ctx            context.Context
		errObj         errors.Error
		err            error
	)

	// build worker monitor key
	monitorKey = common.BuildMonitorKey(ip)

	ctx, _ = utils.GetContextWithTimeout()
	// make lease to notify worker
	// 创建一个租约 让其稍后过期并自动删除
	if leaseGrantResp, err = a.etcd.Lease().Grant(ctx, common.MonitorFrequency+1); err != nil {
		errObj = errors.ErrInternalError
		errObj.Log = "[Etcd - SaveMonitor] lease grant error:" + err.Error()
		return errObj
	}

	ctx, _ = utils.GetContextWithTimeout()
	// save to etcd
	if _, err = a.etcd.KV().Put(ctx, monitorKey, string(monitorInfo), clientv3.WithLease(leaseGrantResp.ID)); err != nil {
		errObj = errors.ErrInternalError
		errObj.Log = "[Etcd - SaveMonitor] etcd client kv put error:" + err.Error()
		return errObj
	}

	return nil
}

// GetMonitor 获取节点的监控信息
func (a *app) GetMonitor(ip string) (*common.MonitorInfo, error) {

	var (
		monitorKey string
		getResp    *clientv3.GetResponse
		monitor    *common.MonitorInfo
		ctx        context.Context
		errObj     errors.Error
		err        error
	)
	// build worker monitor key
	monitorKey = common.BuildMonitorKey(ip)

	ctx, _ = utils.GetContextWithTimeout()

	if getResp, err = a.etcd.KV().Get(ctx, monitorKey); err != nil {
		errObj = errors.ErrInternalError
		errObj.Log = "[Etcd - GetMonitor] etcd client kv get one error:" + err.Error()
		return nil, errObj
	}

	if getResp.Count > 1 {
		errObj = errors.ErrInternalError
		errObj.Log = "[Etcd - GetMonitor] etcd client kv get one task but result > 1"
		return nil, errObj
	} else if getResp.Count == 0 {
		return nil, errors.ErrDataNotFound
	}

	monitor = &common.MonitorInfo{}
	if err = json.Unmarshal(getResp.Kvs[0].Value, monitor); err != nil {
		errObj = errors.ErrInternalError
		errObj.Log = "[Etcd - GetMonitor] monitor json.Unmarshal error:" + err.Error()
		return nil, errObj
	}

	return monitor, nil
}
