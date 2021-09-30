package bolt

import (
	"strings"

	"github.com/juju/errors"
	"github.com/siddontang/go-mysql/mysql"
	"go.etcd.io/bbolt"
	"google.golang.org/protobuf/proto"

	"go-mysql-transfer/domain/po"
	"go-mysql-transfer/domain/vo"
	"go-mysql-transfer/util/log"
)

type PipelineInfoDao struct {
}

func (s *PipelineInfoDao) getBucket(tx *bbolt.Tx) *bbolt.Bucket {
	return tx.Bucket(_pipelineBucket)
}

func (s *PipelineInfoDao) Insert(pipeline *po.PipelineInfo, rules []*po.TransformRule) error {
	return _mdb.Update(func(tx *bbolt.Tx) error {
		data, err := proto.Marshal(pipeline)
		if err != nil {
			return err
		}
		if s.getBucket(tx).Put(marshalId(pipeline.Id), data); err != nil {
			return err
		}

		ruleBucket := tx.Bucket(_ruleBucket)
		for _, rule := range rules {
			d, err := proto.Marshal(rule)
			if err != nil {
				return err
			}
			if ruleBucket.Put(marshalId(rule.Id), d); err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *PipelineInfoDao) UpdateEntity(pipeline *po.PipelineInfo, rules []*po.TransformRule) error {
	return _mdb.Update(func(tx *bbolt.Tx) error {
		// delete rules
		ruleIds := make([]uint64, 0)
		ruleBucket := tx.Bucket(_ruleBucket)
		ruleCursor := ruleBucket.Cursor()
		for k, v := ruleCursor.First(); k != nil; k, v = ruleCursor.Next() {
			var tempRule po.TransformRule
			if err := proto.Unmarshal(v, &tempRule); err == nil {
				if tempRule.PipelineInfoId == pipeline.Id {
					ruleIds = append(ruleIds, tempRule.Id)
				}
			}
		}
		for _, id := range ruleIds {
			if err := ruleBucket.Delete(marshalId(id)); err != nil {
				return err
			}
		}

		// save rules
		for _, rule := range rules {
			d, err := proto.Marshal(rule)
			if err != nil {
				return err
			}
			if ruleBucket.Put(marshalId(rule.Id), d); err != nil {
				return err
			}
		}

		// save PipelineInfo
		pipelineData, err := proto.Marshal(pipeline)
		if err != nil {
			return err
		}
		return s.getBucket(tx).Put(marshalId(pipeline.Id), pipelineData)
	})
}

func (s *PipelineInfoDao) UpdateStatus(id uint64, status uint32) error {
	return _mdb.Update(func(tx *bbolt.Tx) error {
		var entity po.PipelineInfo
		bucket := tx.Bucket(_pipelineBucket)
		idData := marshalId(id)
		entityData := bucket.Get(idData)
		if entityData == nil {
			return errors.NotFoundf("PipelineInfo")
		}
		err := proto.Unmarshal(entityData, &entity)
		if err != nil {
			return err
		}

		entity.Status = status
		entityData, err = proto.Marshal(&entity)
		if err != nil {
			return err
		}
		return bucket.Put(idData, entityData)
	})
}

func (s *PipelineInfoDao) UpdatePosition(id uint64, pos mysql.Position) error {
	return _mdb.Update(func(tx *bbolt.Tx) error {
		var entity po.PipelineInfo
		bucket := tx.Bucket(_pipelineBucket)
		idData := marshalId(id)
		entityData := bucket.Get(idData)
		if entityData == nil {
			return errors.NotFoundf("PipelineInfo")
		}
		err := proto.Unmarshal(entityData, &entity)
		if err != nil {
			return err
		}

		entity.PosName = pos.Name
		entity.PosIndex = pos.Pos
		entityData, err = proto.Marshal(&entity)
		if err != nil {
			return err
		}
		return bucket.Put(idData, entityData)
	})
}

func (s *PipelineInfoDao) Delete(id uint64) error {
	return _mdb.Update(func(tx *bbolt.Tx) error {
		// delete rules
		ruleIds := make([]uint64, 0)
		ruleBucket := tx.Bucket(_ruleBucket)
		ruleCursor := ruleBucket.Cursor()
		for k, v := ruleCursor.First(); k != nil; k, v = ruleCursor.Next() {
			var tempRule po.TransformRule
			if err := proto.Unmarshal(v, &tempRule); err == nil {
				if tempRule.PipelineInfoId == id {
					ruleIds = append(ruleIds, tempRule.Id)
				}
			}
		}
		for _, ruleId := range ruleIds {
			if err := ruleBucket.Delete(marshalId(ruleId)); err != nil {
				return err
			}
		}

		return s.getBucket(tx).Delete(marshalId(id))
	})
}

func (s *PipelineInfoDao) Get(id uint64) (*po.PipelineInfo, error) {
	var entity po.PipelineInfo
	err := _mdb.View(func(tx *bbolt.Tx) error {
		data := s.getBucket(tx).Get(marshalId(id))
		if data == nil {
			return errors.NotFoundf("PipelineInfo")
		}
		return proto.Unmarshal(data, &entity)
	})

	if err != nil {
		return nil, err
	}
	return &entity, err
}

func (s *PipelineInfoDao) GetPosition(id uint64) (mysql.Position, error) {
	var entity po.PipelineInfo
	err := _mdb.View(func(tx *bbolt.Tx) error {
		data := tx.Bucket(_pipelineBucket).Get(marshalId(id))
		if data == nil {
			return errors.NotFoundf("PipelineInfo")
		}
		return proto.Unmarshal(data, &entity)
	})

	if err != nil {
		log.Error(err.Error())
		return mysql.Position{}, err
	}

	return mysql.Position{
		Name: entity.PosName,
		Pos:  entity.PosIndex,
	}, nil
}

func (s *PipelineInfoDao) GetByName(name string) (*po.PipelineInfo, error) {
	var entity po.PipelineInfo
	var found bool
	err := _mdb.View(func(tx *bbolt.Tx) error {
		cursor := tx.Bucket(_pipelineBucket).Cursor()
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			if err := proto.Unmarshal(v, &entity); err == nil {
				if name == entity.Name {
					found = true
					break
				}
			}
		}
		return nil
	})

	if err != nil {
		log.Error(err.Error())
		return nil, err
	}
	if !found {
		return nil, errors.NotFoundf("PipelineInfo")
	}
	return &entity, err
}

func (s *PipelineInfoDao) GetBySourceAndEndpoint(sourceId, endpointId uint64) (*po.PipelineInfo, error) {
	var entity po.PipelineInfo
	var found bool
	err := _mdb.View(func(tx *bbolt.Tx) error {
		cursor := tx.Bucket(_pipelineBucket).Cursor()
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			if err := proto.Unmarshal(v, &entity); err == nil {
				if sourceId == entity.SourceId && endpointId == entity.EndpointId {
					found = true
					break
				}
			}
		}
		return nil
	})

	if err != nil {
		log.Error(err.Error())
		return nil, err
	}
	if !found {
		return nil, errors.NotFoundf("PipelineInfo")
	}
	return &entity, err
}

func (s *PipelineInfoDao) SelectList(params *vo.PipelineInfoParams) ([]*po.PipelineInfo, error) {
	list := make([]*po.PipelineInfo, 0)
	err := _mdb.View(func(tx *bbolt.Tx) error {
		cursor := s.getBucket(tx).Cursor()
		for k, v := cursor.Last(); k != nil; k, v = cursor.Prev() {
			var entity po.PipelineInfo
			if err := proto.Unmarshal(v, &entity); err == nil {
				if params.Name != "" && !strings.Contains(entity.Name, params.Name) {
					continue
				}
				list = append(list, &entity)
			}
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	return list, err
}
