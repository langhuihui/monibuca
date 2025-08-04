package m7s

import (
	"context"
	"net/url"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"
	"m7s.live/v5/pb"
)

type AliasStream struct {
	*Publisher `gorm:"-:all"`
	AutoRemove bool
	StreamPath string
	Alias      string `gorm:"primarykey"`
}

func (a *AliasStream) GetKey() string {
	return a.Alias
}

// StreamAliasDB 用于存储流别名的数据库模型
type StreamAliasDB struct {
	AliasStream
	CreatedAt time.Time `yaml:"-"`
	UpdatedAt time.Time `yaml:"-"`
}

func (StreamAliasDB) TableName() string {
	return "stream_alias"
}

func (s *Server) initStreamAlias() {
	if s.DB == nil {
		return
	}
	var aliases []StreamAliasDB
	s.DB.Find(&aliases)
	for _, alias := range aliases {
		s.AliasStreams.Add(&alias.AliasStream)
		if publisher, ok := s.Streams.Get(alias.StreamPath); ok {
			alias.Publisher = publisher
		}
	}
}

func (s *Server) GetStreamAlias(ctx context.Context, req *emptypb.Empty) (res *pb.StreamAliasListResponse, err error) {
	res = &pb.StreamAliasListResponse{}
	s.CallOnStreamTask(func() {
		for alias := range s.AliasStreams.Range {
			info := &pb.StreamAlias{
				StreamPath: alias.StreamPath,
				Alias:      alias.Alias,
				AutoRemove: alias.AutoRemove,
			}
			if s.Streams.Has(alias.Alias) {
				info.Status = 2
			} else if alias.Publisher != nil {
				info.Status = 1
			}
			res.Data = append(res.Data, info)
		}
	})
	return
}

func (s *Server) SetStreamAlias(ctx context.Context, req *pb.SetStreamAliasRequest) (res *pb.SuccessResponse, err error) {
	res = &pb.SuccessResponse{}
	s.CallOnStreamTask(func() {
		if req.StreamPath != "" {
			u, err := url.Parse(req.StreamPath)
			if err != nil {
				return
			}
			req.StreamPath = strings.TrimPrefix(u.Path, "/")
			publisher, canReplace := s.Streams.Get(req.StreamPath)
			if !canReplace {
				defer s.OnSubscribe(req.StreamPath, u.Query())
			}
			if aliasInfo, ok := s.AliasStreams.Get(req.Alias); ok { //modify alias
				oldStreamPath := aliasInfo.StreamPath
				aliasInfo.AutoRemove = req.AutoRemove
				if aliasInfo.StreamPath != req.StreamPath {
					aliasInfo.StreamPath = req.StreamPath
					if canReplace {
						if aliasInfo.Publisher != nil {
							aliasInfo.TransferSubscribers(publisher) // replace stream
							aliasInfo.Publisher = publisher
						} else {
							aliasInfo.Publisher = publisher
							s.Waiting.WakeUp(req.Alias, publisher)
						}
					}
				}
				// 更新数据库中的别名
				if s.DB != nil {
					s.DB.Where("alias = ?", req.Alias).Assign(aliasInfo).FirstOrCreate(&StreamAliasDB{
						AliasStream: *aliasInfo,
					})
				}
				s.Info("modify alias", "alias", req.Alias, "oldStreamPath", oldStreamPath, "streamPath", req.StreamPath, "replace", ok && canReplace)
			} else { // create alias
				aliasInfo := AliasStream{
					AutoRemove: req.AutoRemove,
					StreamPath: req.StreamPath,
					Alias:      req.Alias,
				}
				var pubId uint32
				s.AliasStreams.Add(&aliasInfo)
				aliasStream, ok := s.Streams.Get(aliasInfo.Alias)
				if canReplace {
					aliasInfo.Publisher = publisher
					if ok {
						aliasStream.TransferSubscribers(publisher) // replace stream
					} else {
						s.Waiting.WakeUp(req.Alias, publisher)
					}
				} else if ok {
					aliasInfo.Publisher = aliasStream
				}
				if aliasInfo.Publisher != nil {
					pubId = aliasInfo.Publisher.ID
				}
				// 保存到数据库
				if s.DB != nil {
					s.DB.Create(&StreamAliasDB{
						AliasStream: aliasInfo,
					})
				}
				s.Info("add alias", "alias", req.Alias, "streamPath", req.StreamPath, "replace", ok && canReplace, "pub", pubId)
			}
		} else {
			s.Info("remove alias", "alias", req.Alias)
			if aliasStream, ok := s.AliasStreams.Get(req.Alias); ok {
				s.AliasStreams.Remove(aliasStream)
				// 从数据库中删除
				if s.DB != nil {
					s.DB.Where("alias = ?", req.Alias).Delete(&StreamAliasDB{})
				}
				if aliasStream.Publisher != nil {
					if publisher, hasTarget := s.Streams.Get(req.Alias); hasTarget { // restore stream
						aliasStream.TransferSubscribers(publisher)
					} else {
						var args url.Values
						for sub := range aliasStream.Publisher.SubscriberRange {
							if sub.StreamPath == req.Alias {
								aliasStream.Publisher.RemoveSubscriber(sub)
								s.Waiting.Wait(sub)
								args = sub.Args
							}
						}
						if args != nil {
							s.OnSubscribe(req.Alias, args)
						}
					}
				}
			}
		}
	})
	return
}

func (p *Publisher) processAliasOnStart() {
	s := p.Plugin.Server
	for alias := range s.AliasStreams.Range {
		if alias.StreamPath != p.StreamPath {
			continue
		}
		if alias.Publisher == nil {
			alias.Publisher = p
			s.Waiting.WakeUp(alias.Alias, p)
		} else if alias.Publisher.StreamPath != alias.StreamPath {
			alias.Publisher.TransferSubscribers(p)
			alias.Publisher = p
		}
	}
}

func (p *Publisher) processAliasOnDispose() {
	s := p.Plugin.Server
	var relatedAlias []*AliasStream
	for alias := range s.AliasStreams.Range {
		if alias.StreamPath == p.StreamPath {
			if alias.AutoRemove {
				defer s.AliasStreams.Remove(alias)
				if s.DB != nil {
					defer s.DB.Where("alias = ?", alias.Alias).Delete(&StreamAliasDB{})
				}
			}
			alias.Publisher = nil
			relatedAlias = append(relatedAlias, alias)
		}
	}
	if p.Subscribers.Length > 0 {
	SUBSCRIBER:
		for subscriber := range p.SubscriberRange {
			for _, alias := range relatedAlias {
				if subscriber.StreamPath == alias.Alias {
					if originStream, ok := s.Streams.Get(alias.Alias); ok {
						originStream.AddSubscriber(subscriber)
						continue SUBSCRIBER
					}
				}
			}
			s.Waiting.Wait(subscriber)
		}
		p.Subscribers.Clear()
	}
}

func (s *Subscriber) processAliasOnStart() (hasInvited bool, done bool) {
	server := s.Plugin.Server
	if alias, ok := server.AliasStreams.Get(s.StreamPath); ok {
		if alias.Publisher != nil {
			alias.Publisher.AddSubscriber(s)
			done = true
			return
		} else {
			server.OnSubscribe(alias.StreamPath, s.Args)
			hasInvited = true
		}
	} else {
		for reg, alias := range server.StreamAlias {
			if streamPath := reg.Replace(s.StreamPath, alias); streamPath != "" {
				as := AliasStream{
					StreamPath: streamPath,
					Alias:      s.StreamPath,
				}
				server.AliasStreams.Set(&as)
				if server.DB != nil {
					server.DB.Where("alias = ?", s.StreamPath).Assign(as).FirstOrCreate(&StreamAliasDB{
						AliasStream: as,
					})
				}
				if publisher, ok := server.Streams.Get(streamPath); ok {
					publisher.AddSubscriber(s)
					done = true
					return
				} else {
					server.OnSubscribe(streamPath, s.Args)
					hasInvited = true
				}
				break
			}
		}
	}
	return
}
