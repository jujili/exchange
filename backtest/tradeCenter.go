package backtest

import (
	"context"
	"log"

	"github.com/ThreeDotsLabs/watermill/message"
	"github.com/jujili/exch"
)

// REVIEW: 要不要把 Pubsub 移动到别的地方去，比如 jujili/jili

// Pubsub is a combination of interface of watermill
type Pubsub interface {
	Publisher
	Subscriber
}

// Publisher is publish interface of watermill
type Publisher interface {
	Publish(topic string, messages ...*message.Message) error
	Close() error
}

// Subscriber is publish interface of watermill
type Subscriber interface {
	Subscribe(ctx context.Context, topic string) (<-chan *message.Message, error)
}

// BackTest 是一个模拟的交易中心
type BackTest struct {
}

// NewBackTest returns a new trade center - bt
// bt subscribe "tick", "order" and "cancelAllOrders" topics from pubsub
// and
// bt publish "balance" topic
//
// TODO: 在成交的时候，还需要发布 "traded" 具体的成交信息。
func NewBackTest(ctx context.Context, ps Pubsub, balance exch.Balance) {
	sells := newOrderList()
	buys := newOrderList()

	ticks, err := ps.Subscribe(ctx, "tick")
	if err != nil {
		panic(err)
	}

	// bars, err := ps.Subscribe(ctx, "bar")
	// if err != nil {
	// 	panic(err)
	// }

	orders, err := ps.Subscribe(ctx, "order")
	if err != nil {
		panic(err)
	}

	// REVIEW:还没有想好如何在回测的时候，维护好策略和回测中心两边的订单。
	// 以便于删除单个订单。
	// 所以，就只好全部都删除了算了。
	// 但是全部删除也不是什么坏事。
	// cancelAllOrders, err := ps.Subscribe(ctx, "cancelAllOrders")
	// if err != nil {
	// panic(err)
	// }

	decOrder := decOrderFunc()
	decTick := exch.DecTickFunc()

	go func() {
		bm := newBalanceManager(ps, balance)
		// 空更新一下，是为了能够让 balanceService 可以获取到 Balance 的数值
		bm.update([]exch.Asset{}...)
		count := 0
		for count < 2 {
			select {
			case <-ctx.Done():
				log.Println("ctx.Done", ctx.Err())
			case msg, ok := <-ticks:
				if !ok {
					count++
					ticks = nil
					continue
				}
				tick := decTick(msg.Payload)
				msg.Ack()
				as := make([]exch.Asset, 0, 32)
				if !buys.isEmpty() {
					// log.Println("before match", buys, tick)
					as = append(as, buys.match(tick)...)
					// log.Println("After  match", buys)
				}
				if !sells.isEmpty() {
					// log.Println("before match", sells, tick)
					as = append(as, sells.match(tick)...)
					// log.Println("After  match", sells)
				}
				if len(as) > 0 {
					// 收取手续费
					fee := 0.001 // 交易手续费
					keep := 1 - fee
					for i, a := range as {
						as[i] = exch.NewAsset(a.Name, a.Free*keep, a.Locked*keep)
					}
					bm.update(as...)
				}
			case msg, ok := <-orders:
				if !ok {
					count++
					orders = nil
					continue
				}
				order := decOrder(msg.Payload)
				msg.Ack()
				if order.Side == exch.BUY {
					bm.update(buys.push(order))
				} else {
					bm.update(sells.push(order))
				}
				// TODO: 添加取消订单的功能
				// case msg := <-cancelAllOrders:
				// msg.Ack()
				// for !buys.isEmpty() {
				// bm.update(buys.pop().cancel2Free())
				// }
				// for !sells.isEmpty() {
				// bm.update(sells.pop().cancel2Free())
				// }
			}
		}
		log.Println("backtest center is over")
	}()
}
