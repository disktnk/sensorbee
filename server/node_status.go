package server

import (
	"github.com/gocraft/web"
	"gopkg.in/sensorbee/sensorbee.v0/server/response"
)

type nodeStatus struct {
	*APIContext
}

func setUpNodeStatusRouter(prefix string, router *web.Router) {
	root := router.Subrouter(nodeStatus{}, "")
	root.Get("/node_status", (*nodeStatus).Show)
}

func (ns *nodeStatus) Show(rw web.ResponseWriter, req *web.Request) {
	tpls, err := ns.topologies.List()
	if err != nil {
		ns.ErrLog(err).Errorf("Cannot get topology list")
		return
	}

	topologies := []interface{}{}
	for tplName, tplBuilder := range tpls {
		tplMap := map[string]interface{}{}
		tplMap["name"] = tplName

		srcs := []interface{}{}
		for _, sn := range tplBuilder.Topology().Sources() {
			srcs = append(srcs, response.NewSource(sn, true))
		}
		tplMap["sources"] = srcs

		boxes := []interface{}{}
		for _, bn := range tplBuilder.Topology().Boxes() {
			boxes = append(boxes, response.NewStream(bn, true))
		}
		tplMap["boxes"] = boxes

		sinks := []interface{}{}
		for _, sn := range tplBuilder.Topology().Sinks() {
			sinks = append(sinks, response.NewSink(sn, true))
		}
		tplMap["sinks"] = sinks

		topologies = append(topologies, tplMap)
	}

	ns.Render(map[string]interface{}{
		"topologies": topologies,
	})
}
